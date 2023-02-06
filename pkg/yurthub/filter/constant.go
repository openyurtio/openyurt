/*
Copyright 2021 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filter

const (
	// MasterServiceFilterName filter is used to mutate the ClusterIP and https port of default/kubernetes service
	// in order to pods on edge nodes can access kube-apiserver directly by inClusterConfig.
	MasterServiceFilterName = "masterservice"

	// ServiceTopologyFilterName filter is used to reassemble endpointslice in order to make the service traffic
	// under the topology that defined by service.Annotation["openyurt.io/topologyKeys"]
	ServiceTopologyFilterName = "servicetopology"

	// DiscardCloudServiceFilterName filter is used to discard cloud service(like loadBalancer service)
	// on kube-proxy list/watch service request from edge nodes.
	DiscardCloudServiceFilterName = "discardcloudservice"

	// InClusterConfigFilterName filter is used to comment kubeconfig in kube-system/kube-proxy configmap
	// in order to make kube-proxy to use InClusterConfig to access kube-apiserver.
	InClusterConfigFilterName = "inclusterconfig"

	// NodePortIsolationName filter is used to discard or keep NodePort service in specified NodePool
	// in order to make NodePort will not be listened by kube-proxy component in specified NodePool.
	NodePortIsolationName = "nodeportisolation"

	// SkipDiscardServiceAnnotation is annotation used by LB service.
	// If end users want to use specified LB service at the edge side,
	// End users should add annotation["openyurt.io/skip-discard"]="true" for LB service.
	SkipDiscardServiceAnnotation = "openyurt.io/skip-discard"
)

var (
	// DisabledInCloudMode contains the filters that should be disabled when yurthub is working in cloud mode.
	DisabledInCloudMode = []string{DiscardCloudServiceFilterName}

	// SupportedComponentsForFilter is used for specifying which components are supported by filters as default setting.
	SupportedComponentsForFilter = map[string]string{
		MasterServiceFilterName:       "kubelet",
		DiscardCloudServiceFilterName: "kube-proxy",
		ServiceTopologyFilterName:     "kube-proxy, coredns, nginx-ingress-controller",
		InClusterConfigFilterName:     "kubelet",
		NodePortIsolationName:         "kube-proxy",
	}
)
