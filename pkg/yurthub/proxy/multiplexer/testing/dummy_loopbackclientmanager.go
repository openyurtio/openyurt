package testing

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type DummyLoopbackClientManager struct {
	RestClient rest.Interface
	Err        error
}

func (d *DummyLoopbackClientManager) RestClientWithGVR(gvr *schema.GroupVersionResource) (rest.Interface, error) {
	return d.RestClient, d.Err
}
