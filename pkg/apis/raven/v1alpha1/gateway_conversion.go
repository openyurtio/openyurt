/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
)

/*
Implementing the hub method is pretty easy -- we just have to add an empty
method called Hub() to serve as a
[marker](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/conversion?tab=doc#Hub).
*/

// NOTE !!!!!! @kadisi
// If this version is storageversion, you only need to uncommand this method

// Hub marks this type as a conversion hub.
//func (*Gateway) Hub() {}

// NOTE !!!!!!! @kadisi
// If this version is not storageversion, you need to implement the ConvertTo and ConvertFrom methods

func (src *Gateway) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Gateway)
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.NodeSelector != nil {
		dst.Spec.NodeSelector = src.Spec.NodeSelector
	}
	dst.Spec.ExposeType = string(src.Spec.ExposeType)
	dst.Spec.TunnelConfig.Replicas = 1
	dst.Spec.ProxyConfig.Replicas = 1
	for _, eps := range src.Spec.Endpoints {
		dst.Spec.Endpoints = append(dst.Spec.Endpoints, v1beta1.Endpoint{
			NodeName: eps.NodeName,
			PublicIP: eps.PublicIP,
			UnderNAT: eps.UnderNAT,
			Config:   eps.Config,
			Type:     v1beta1.Tunnel,
			Port:     v1beta1.DefaultTunnelServerExposedPort,
		})
	}
	for _, node := range src.Status.Nodes {
		dst.Status.Nodes = append(dst.Status.Nodes, v1beta1.NodeInfo{
			NodeName:  node.NodeName,
			PrivateIP: node.PrivateIP,
			Subnets:   node.Subnets,
		})
	}
	if src.Status.ActiveEndpoint != nil {
		dst.Status.ActiveEndpoints = []*v1beta1.Endpoint{
			{
				NodeName: src.Status.ActiveEndpoint.NodeName,
				PublicIP: src.Status.ActiveEndpoint.PublicIP,
				UnderNAT: src.Status.ActiveEndpoint.UnderNAT,
				Config:   src.Status.ActiveEndpoint.Config,
				Type:     v1beta1.Tunnel,
				Port:     v1beta1.DefaultTunnelServerExposedPort,
			},
		}
	}

	klog.Infof("convert from v1alpha1  to v1beta1 for %s", dst.Name)
	return nil
}

// NOTE !!!!!!! @kadisi
// If this version is not storageversion, you need to implement the ConvertTo and ConvertFrom methods

func (dst *Gateway) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.Gateway)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.NodeSelector = src.Spec.NodeSelector
	dst.Spec.ExposeType = ExposeType(src.Spec.ExposeType)
	for _, eps := range src.Spec.Endpoints {
		dst.Spec.Endpoints = append(dst.Spec.Endpoints, Endpoint{
			NodeName: eps.NodeName,
			PublicIP: eps.PublicIP,
			UnderNAT: eps.UnderNAT,
			Config:   eps.Config,
		})
	}
	for _, node := range src.Status.Nodes {
		dst.Status.Nodes = append(dst.Status.Nodes, NodeInfo{
			NodeName:  node.NodeName,
			PrivateIP: node.PrivateIP,
			Subnets:   node.Subnets,
		})
	}
	if src.Status.ActiveEndpoints == nil {
		klog.Infof("convert from v1beta1 to v1alpha1 for %s", dst.Name)
		return nil
	}
	if len(src.Status.ActiveEndpoints) < 1 {
		dst.Status.ActiveEndpoint = nil
	} else {
		dst.Status.ActiveEndpoint = &Endpoint{
			NodeName: src.Status.ActiveEndpoints[0].NodeName,
			PublicIP: src.Status.ActiveEndpoints[0].PublicIP,
			UnderNAT: src.Status.ActiveEndpoints[0].UnderNAT,
			Config:   src.Status.ActiveEndpoints[0].Config,
		}
	}
	klog.Infof("convert from v1beta1 to v1alpha1 for %s", dst.Name)
	return nil
}
