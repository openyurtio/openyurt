package endpointsfilter

import (
	"bytes"
	"context"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func TestObjectResponseFilter(t *testing.T) {
	currentNodeName := "node1"
	node2 := "node2"
	node3 := "node3"

	testcases := map[string]struct {
		kubeClient   *k8sfake.Clientset
		yurtClient   *yurtfake.Clientset
		originalList runtime.Object
		expectResult runtime.Object
	}{
		"endpoints node name in node pool": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
		"endpoints node name not in node pool": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &node3,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node3,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{},
		},
		"no node name in addresses of endpoints subsets": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP: "123.223.0.1",
									},
									{
										IP: "123.223.0.2",
									},
									{
										IP: "123.223.0.3",
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{},
		},
		"node pool not exists": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "not-exist",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
		"current node hasn't node pool label": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
		"no current node in cluster": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
		"no service for endpoints": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.EndpointsList{
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
							},
						},
						Subsets: []corev1.EndpointSubset{
							{
								Addresses: []corev1.EndpointAddress{
									{
										IP:       "123.223.0.1",
										NodeName: &currentNodeName,
									},
									{
										IP:       "123.223.0.2",
										NodeName: &node2,
									},
									{
										IP:       "123.223.0.3",
										NodeName: &node3,
									},
								},
								Ports: []corev1.EndpointPort{
									{
										Name:     "https",
										Port:     6443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
		"not endpointsList": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			originalList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {

			factory := informers.NewSharedInformerFactory(tt.kubeClient, 24*time.Hour)
			serviceInformer := factory.Core().V1().Services()
			serviceInformer.Informer()
			serviceLister := serviceInformer.Lister()

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			yurtFactory := yurtinformers.NewSharedInformerFactory(tt.yurtClient, 24*time.Hour)
			nodePoolInformer := yurtFactory.Apps().V1alpha1().NodePools()
			nodePoolLister := nodePoolInformer.Lister()

			stopper2 := make(chan struct{})
			defer close(stopper2)
			yurtFactory.Start(stopper2)
			yurtFactory.WaitForCacheSync(stopper2)

			nodeGetter := func(name string) (*corev1.Node, error) {
				return tt.kubeClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
			}

			fh := &endpointsFilterHandler{
				nodeName:       currentNodeName,
				serviceLister:  serviceLister,
				nodePoolLister: nodePoolLister,
				nodeGetter:     nodeGetter,
				serializer: serializer.NewSerializerManager().
					CreateSerializer("application/json", "", "v1", "endpoints"),
			}

			originalBytes, err := fh.serializer.Encode(tt.originalList)
			if err != nil {
				t.Errorf("encode originalList error: %v\n", err)
			}

			filteredBytes, err := fh.ObjectResponseFilter(originalBytes)
			if err != nil {
				t.Errorf("ObjectResponseFilter got error: %v\n", err)
			}

			expectedBytes, err := fh.serializer.Encode(tt.expectResult)
			if err != nil {
				t.Errorf("encode expectedResult error: %v\n", err)
			}

			if !bytes.Equal(filteredBytes, expectedBytes) {
				result, _ := fh.serializer.Decode(filteredBytes)
				t.Errorf("ObjectResponseFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult, result)
			}
		})
	}
}

func TestStreamResponseFilter(t *testing.T) {
	currentNodeName := "node1"
	node2 := "node2"
	node3 := "node3"

	testcases := map[string]struct {
		kubeClient   *k8sfake.Clientset
		yurtClient   *yurtfake.Clientset
		inputObj     []watch.Event
		expectResult []runtime.Object
	}{
		"endpoints node name in node pool": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &currentNodeName,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node2,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						filter.SkipDiscardServiceAnnotation: "true",
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "123.223.0.1",
								NodeName: &currentNodeName,
							},
							{
								IP:       "123.223.0.2",
								NodeName: &node2,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "https",
								Port:     6443,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}},
		},
		"endpoints node name not in node pool": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &node3,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node3,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{},
		},
		"no node name in addresses of endpoints subsets": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP: "123.223.0.1",
								},
								{
									IP: "123.223.0.2",
								},
								{
									IP: "123.223.0.3",
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{},
		},
		"node pool not exists": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "not-exist",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &currentNodeName,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node2,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						filter.SkipDiscardServiceAnnotation: "true",
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "123.223.0.1",
								NodeName: &currentNodeName,
							},
							{
								IP:       "123.223.0.2",
								NodeName: &node2,
							},
							{
								IP:       "123.223.0.3",
								NodeName: &node3,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "https",
								Port:     6443,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}},
		},
		"current node hasn't node pool label": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &currentNodeName,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node2,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						filter.SkipDiscardServiceAnnotation: "true",
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "123.223.0.1",
								NodeName: &currentNodeName,
							},
							{
								IP:       "123.223.0.2",
								NodeName: &node2,
							},
							{
								IP:       "123.223.0.3",
								NodeName: &node3,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "https",
								Port:     6443,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}},
		},
		"no current node in cluster": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &currentNodeName,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node2,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						filter.SkipDiscardServiceAnnotation: "true",
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "123.223.0.1",
								NodeName: &currentNodeName,
							},
							{
								IP:       "123.223.0.2",
								NodeName: &node2,
							},
							{
								IP:       "123.223.0.3",
								NodeName: &node3,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "https",
								Port:     6443,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}},
		},
		"no service for endpoints": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "123.223.0.1",
									NodeName: &currentNodeName,
								},
								{
									IP:       "123.223.0.2",
									NodeName: &node2,
								},
								{
									IP:       "123.223.0.3",
									NodeName: &node3,
								},
							},
							Ports: []corev1.EndpointPort{
								{
									Name:     "https",
									Port:     6443,
									Protocol: corev1.ProtocolTCP,
								},
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						filter.SkipDiscardServiceAnnotation: "true",
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "123.223.0.1",
								NodeName: &currentNodeName,
							},
							{
								IP:       "123.223.0.2",
								NodeName: &node2,
							},
							{
								IP:       "123.223.0.3",
								NodeName: &node3,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "https",
								Port:     6443,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}},
		},
		"not endpointsList": {
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: node3,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node3,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							node2,
						},
					},
				},
			),
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx",
						},
					},
				},
			}},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {

			factory := informers.NewSharedInformerFactory(tt.kubeClient, 24*time.Hour)
			serviceInformer := factory.Core().V1().Services()
			serviceInformer.Informer()
			serviceLister := serviceInformer.Lister()

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			yurtFactory := yurtinformers.NewSharedInformerFactory(tt.yurtClient, 24*time.Hour)
			nodePoolInformer := yurtFactory.Apps().V1alpha1().NodePools()
			nodePoolLister := nodePoolInformer.Lister()

			stopper2 := make(chan struct{})
			defer close(stopper2)
			yurtFactory.Start(stopper2)
			yurtFactory.WaitForCacheSync(stopper2)

			nodeGetter := func(name string) (*corev1.Node, error) {
				return tt.kubeClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
			}

			fh := &endpointsFilterHandler{
				nodeName:       currentNodeName,
				serviceLister:  serviceLister,
				nodePoolLister: nodePoolLister,
				nodeGetter:     nodeGetter,
				serializer: serializer.NewSerializerManager().
					CreateSerializer("application/json", "", "v1", "endpoints"),
			}

			r, w := io.Pipe()
			go func(w *io.PipeWriter) {
				for i := range tt.inputObj {
					if _, err := fh.serializer.WatchEncode(w, &tt.inputObj[i]); err != nil {
						t.Errorf("%d: encode watch unexpected error: %v", i, err)
						continue
					}
					time.Sleep(100 * time.Millisecond)
				}
				w.Close()
			}(w)

			rc := io.NopCloser(r)
			ch := make(chan watch.Event, len(tt.inputObj))

			go func(rc io.ReadCloser, ch chan watch.Event) {
				fh.StreamResponseFilter(rc, ch)
			}(rc, ch)

			for i := 0; i < len(tt.expectResult); i++ {
				event := <-ch

				resultBytes, _ := fh.serializer.Encode(event.Object)
				expectedBytes, _ := fh.serializer.Encode(tt.expectResult[i])

				if !bytes.Equal(resultBytes, expectedBytes) {
					t.Errorf("StreamResponseFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult[i], event.Object)
					break
				}
			}
		})
	}
}
