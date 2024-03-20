package loadbalancer

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
)

type ServiceFinalizer struct {
	client client.Client
}

var _ finalizer.Finalizer = &ServiceFinalizer{}

func NewLoadBalancerServiceFinalizer(cli client.Client) *ServiceFinalizer {
	return &ServiceFinalizer{client: cli}
}

func (l ServiceFinalizer) Finalize(ctx context.Context, object client.Object) (finalizer.Result, error) {
	var res finalizer.Result
	_, ok := object.(*corev1.Service)
	if !ok {
		res.Updated = false
		return res, fmt.Errorf("object is not corev1.service")
	}
	res.Updated = true
	return res, nil
}

func Finalize(ctx context.Context, obj client.Object, cli client.Client, flz finalizer.Finalizer) error {
	oldObj := obj.DeepCopyObject().(client.Object)
	res, err := flz.Finalize(ctx, obj)
	if err != nil {
		return err
	}
	if res.Updated {
		return cli.Patch(ctx, obj, client.MergeFrom(oldObj))
	}
	return nil
}

// HasFinalizer tests whether k8s object has specified finalizer
func HasFinalizer(obj client.Object, finalizer string) bool {
	f := obj.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}
