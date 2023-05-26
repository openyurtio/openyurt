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
	"context"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *GatewayHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	gw, ok := obj.(*v1alpha1.Gateway)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Gateway but got a %T", obj))
	}

	return validate(gw)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *GatewayHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newGw, ok := newObj.(*v1alpha1.Gateway)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Gateway but got a %T", newObj))
	}
	_, ok = oldObj.(*v1alpha1.Gateway)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Gateway} but got a %T", oldObj))
	}
	if err := validate(newGw); err != nil {
		return err
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *GatewayHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	gw, ok := obj.(*v1alpha1.Gateway)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Gateway but got a %T", obj))
	}
	return validate(gw)
}

func validate(g *v1alpha1.Gateway) error {
	var errList field.ErrorList

	if g.Spec.ExposeType != "" {
		if g.Spec.ExposeType != v1alpha1.ExposeTypeLoadBalancer && g.Spec.ExposeType != v1alpha1.ExposeTypePublicIP {
			fldPath := field.NewPath("spec").Child("exposeType")
			errList = append(errList, field.Invalid(fldPath, g.Spec.ExposeType, "the 'exposeType' field  is irregularity"))
		}
		if g.Spec.ExposeType == v1alpha1.ExposeTypeLoadBalancer || g.Spec.ExposeType == v1alpha1.ExposeTypePublicIP {
			for i, ep := range g.Spec.Endpoints {
				if ep.UnderNAT {
					fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("underNAT")
					errList = append(errList, field.Invalid(fldPath, ep.UnderNAT, fmt.Sprintf("the 'underNAT' field for exposed gateway %s/%s must be false", g.Namespace, g.Name)))
				}
			}
		}
	}

	if len(g.Spec.Endpoints) == 0 {
		return nil
	}
	underNAT := g.Spec.Endpoints[0].UnderNAT
	for i, ep := range g.Spec.Endpoints {
		if ep.UnderNAT != underNAT {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("underNAT")
			errList = append(errList, field.Invalid(fldPath, ep.UnderNAT, "the 'underNAT' field in endpoints must be the same"))
		}

		if ep.PublicIP != "" {
			if err := validateIP(ep.PublicIP); err != nil {
				fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("publicIP")
				errList = append(errList, field.Invalid(fldPath, ep.PublicIP, "the 'publicIP' field must be a validate IP address"))
			}
		}

		if ep.Type == "" {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("type")
			errList = append(errList, field.Invalid(fldPath, ep.Type, "the 'type' field must not be empty"))
		} else {
			if ep.Type != v1alpha1.Tunnel && ep.Type != v1alpha1.Proxy {
				fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("type")
				errList = append(errList, field.Invalid(fldPath, ep.Type, fmt.Sprintf("the 'type' field must be set %s or %s ", v1alpha1.Tunnel, v1alpha1.Proxy)))
			}
		}

		if ep.Type == v1alpha1.Tunnel && ep.Port != v1alpha1.TunnelPort {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("port")
			errList = append(errList, field.Invalid(fldPath, ep.Port, fmt.Sprintf("the 'port' field must be set %d for type %s ", v1alpha1.TunnelPort, v1alpha1.Tunnel)))
		}
		if ep.Type == v1alpha1.Proxy && ep.Port != v1alpha1.ProxyPort {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("port")
			errList = append(errList, field.Invalid(fldPath, ep.Port, fmt.Sprintf("the 'port' field must be set %d for type %s ", v1alpha1.TunnelPort, v1alpha1.Proxy)))
		}

		if len(ep.NodeName) == 0 {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("nodeName")
			errList = append(errList, field.Invalid(fldPath, ep.NodeName, "the 'nodeName' field must not be empty"))
		}
	}

	if errList != nil {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: v1alpha1.SchemeGroupVersion.Group, Kind: g.Kind},
			g.Name, errList)
	}

	klog.Infof("Validate Gateway %s successfully ...", klog.KObj(g))

	return nil
}

func validateIP(ip string) error {
	s := net.ParseIP(ip)
	if s.To4() != nil || s.To16() != nil {
		return nil
	}
	return fmt.Errorf("invalid ip address: %s", ip)
}
