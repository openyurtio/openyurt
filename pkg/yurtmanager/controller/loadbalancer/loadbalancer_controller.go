package loadbalancer

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/driver"
	sharedContext "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/sharedcontext"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", name, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r, err := newReconciler(ctx, c, mgr)
	if err != nil {
		klog.Error(Format("new reconcile error: %s", err.Error()))
		return err
	}
	return add(mgr, r)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *Reconcile) error {
	rateLimit := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Second, 300*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)

	c, err := controller.New(name, mgr,
		controller.Options{Reconciler: r,
			MaxConcurrentReconciles: 2,
			RateLimiter:             rateLimit,
			RecoverPanic:            true,
		})
	if err != nil {
		return err
	}
	if err = c.Watch(&source.Kind{Type: &v1.Service{}},
		&handler.EnqueueRequestForObject{},
		NewPredictionForServiceEvent(mgr.GetClient(), r.drivers),
	); err != nil {
		return fmt.Errorf("watch resource svc error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &discovery.EndpointSlice{}},
		NewEnqueueRequestForEndpointSliceEvent(),
		NewPredictionForEndpointSliceEvent(mgr.GetClient(), r.drivers),
	); err != nil {
		return fmt.Errorf("watch resource endpointslice error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &v1.Node{}},
		NewEnqueueRequestForNodeEvent(mgr.GetClient(), r.drivers),
		NewPredictionForNodeEvent(mgr.GetClient(), r.drivers),
	); err != nil {
		return fmt.Errorf("watch resource nodes error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &nodepoolv1beta1.NodePool{}},
		NewEnqueueRequestForNodePoolEvent(mgr.GetClient(), r.drivers),
		NewPredictionForNodePoolEvent(mgr.GetClient(), r.drivers),
	); err != nil {
		return fmt.Errorf("watch resource nodepools error: %s", err.Error())
	}

	for key := range r.drivers {
		if err := r.drivers[key].Init(); err != nil {
			return fmt.Errorf("init driver %s error: %s", key, err.Error())
		}
	}
	return nil
}

// ReconcileService implements reconcile.Reconciler
var _ reconcile.Reconciler = &Reconcile{}

// ReconcileELB reconciles a AutoRepair object
type Reconcile struct {
	scheme    *runtime.Scheme
	client    client.Client
	record    record.EventRecorder
	finalizer finalizer.Finalizers
	drivers   driver.Drivers
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) (*Reconcile, error) {
	drivers, err := driver.NewDrivers(c, mgr)
	if err != nil {
		return nil, err
	}
	recon := &Reconcile{
		client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		record:    mgr.GetEventRecorderFor(name),
		finalizer: finalizer.NewFinalizers(),
		drivers:   drivers,
	}
	err = recon.finalizer.Register(LBFinalizer, NewLoadBalancerServiceFinalizer(mgr.GetClient()))
	if err != nil {
		klog.Info("new finalizer error %s, can ignore it", err.Error())
	}
	return recon, nil
}

func (r Reconcile) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Info(Format("reconcile: start reconcile service %v", request.NamespacedName))
	defer klog.Info(Format("successfully reconcile service %v", request.NamespacedName))
	svc := &v1.Service{}
	err := r.client.Get(context.Background(), request.NamespacedName, svc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("service not found, skip ", "service: ", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		klog.Error("reconcile: get service failed", "service", request.NamespacedName, "error", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}
	if needDeleteLoadBalancer(svc) {
		err = r.cleanupLoadBalancer(ctx, svc)
	} else {
		err = r.applyLoadBalancer(ctx, svc)
	}
	if err != nil {
		klog.Error("reconcile: %s", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r *Reconcile) cleanupLoadBalancer(ctx context.Context, svc *v1.Service) error {
	if HasFinalizer(svc, LBFinalizer) {
		// 1.cleanup loadbalancer configuration
		drv, ok := r.drivers[GetLoadBalancerClass(svc)]
		if !ok {
			klog.Infof("failed to handle the type %s of service", GetLoadBalancerClass(svc))
			return nil
		}

		// 2. list current status of loadbalancers
		lbs, err := drv.List(ctx, svc)
		for _, lb := range lbs {
			err := drv.Cleanup(ctx, svc, lb.PoolName())
		}

		// 2.remove labels
		if err = r.RemoveLabelsAndAnnotations(reqCtx, pools); err != nil {
			return fmt.Errorf("%s failed to add labels and annotations, error: %s", Key(reqCtx.Service), err.Error())
		}

		// 3.remove status
		if err = r.RemoveStatus(reqCtx, pools); err != nil {
			return fmt.Errorf("%s failed to update status, error: %s", Key(reqCtx.Service), err.Error())
		}
		// 4.remove finalizer
		// 1. add finalizer
		if err := Finalize(reqCtx.Ctx, reqCtx.Service, r.client, r.finalizer); err != nil {
			r.record.Event(reqCtx.Service, v1.EventTypeWarning, FailedRemoveFinalizer,
				fmt.Sprintf("Error removing finalizer: %s", err.Error()))
			return fmt.Errorf("%s failed to remove service finalizer, error: %s", Key(reqCtx.Service), err.Error())
		}
	}
	return nil
}

func (r *Reconcile) applyLoadBalancer(ctx context.Context, svc *v1.Service) error {
	// 1. add finalizer
	if err := Finalize(ctx, svc, r.client, r.finalizer); err != nil {
		r.record.Event(svc, v1.EventTypeWarning, FailedAddFinalizer,
			fmt.Sprintf("Error adding finalizer: %s", err.Error()))
		return fmt.Errorf("%s failed to add service finalizer, error: %s", Key(svc))
	}

	// 2. get driver
	drv, ok := r.drivers[GetLoadBalancerClass(svc)]
	if !ok {
		klog.Infof("failed to handle the type %s of service", GetLoadBalancerClass(svc))
		return nil
	}

	//4.list current loadbalancer
	prev, err := drv.List(ctx, svc)
	curr, err := currentLoadBalancers(ctx, svc)
	add, updae, delete := classifyLoadBalancer(prev, curr)
	res := make([]driver.LoadBalancer, 0)
	for _, lb := range lbs {
		switch lb.Action() {
		case driver.Update, driver.Create:
			l, err := drv.Ensure(ctx, svc, lb.PoolName())
			if err == nil {
			}
			res = append(res, l)
		case driver.Delete:
			err := drv.Cleanup(ctx, svc, lb.PoolName())
		default:
		}
	}
	labels, annotations := drv.ResolveLoadBalancerLabelsAndAnnotations(ctx, res)
	// 3. add labels
	if err = r.AddLabelsAndAnnotations(ctx, svc, labels, annotations); err != nil {
		return fmt.Errorf("%s failed to add labels and annotations, error: %s", Key(svc), err.Error())
	}

	accesses := make([]driver.AccessPoint, 0)
	for _, l := range res {
		accesses = append(accesses, l.AccessPoints()...)
	}
	// 4. update status
	if err = r.UpdateStatus(ctx, svc, accesses); err != nil {
		return fmt.Errorf("%s failed to update status, error: %s", Key(svc), err.Error())
	}
	return nil
}

func currentLoadBalancers(ctx context.Context, svc *v1.Service) ([]driver.LoadBalancer, error) {
	return nil, nil
}

func classifyLoadBalancer(prev, curr []driver.LoadBalancer) []driver.LoadBalancer {
	return nil
}
func (r *Reconcile) AddLabelsAndAnnotations(ctx context.Context, svc *v1.Service, labels, annotations map[string]string) error {
	return nil
}

func (r *Reconcile) UpdateStatus(ctx context.Context, svc *v1.Service, access []driver.AccessPoint) error {
	return nil
}

func (r *Reconcile) RemoveLabelsAndAnnotations(reqCtx *sharedContext.RequestContext, pools map[string]*driver.Configuration) error {
	return nil
}

func (r *Reconcile) RemoveStatus(reqCtx *sharedContext.RequestContext, pools map[string]*driver.Configuration) error {
	return nil
}
