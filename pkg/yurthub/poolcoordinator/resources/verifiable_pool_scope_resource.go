package resources

import "k8s.io/apimachinery/pkg/runtime/schema"

type verifiablePoolScopeResource struct {
	schema.GroupVersionResource
	checkFunction func(gvr schema.GroupVersionResource) (bool, string)
}

func newVerifiablePoolScopeResource(gvr schema.GroupVersionResource,
	checkFunction func(gvr schema.GroupVersionResource) (bool, string)) *verifiablePoolScopeResource {
	return &verifiablePoolScopeResource{
		GroupVersionResource: gvr,
		checkFunction:        checkFunction,
	}
}

func (v *verifiablePoolScopeResource) Verify() (bool, string) {
	return v.checkFunction(v.GroupVersionResource)
}
