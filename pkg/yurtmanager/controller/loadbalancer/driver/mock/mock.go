package mock

import (
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/driver"
	reqCtx "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/sharedcontext"
)

const MockClass = "openyurt.io/mock"

type provider interface{}

func NewProvider(path string) provider {
	return nil
}

type LBManager struct{}
type ListenerManager struct{}
type ServerGroupManager struct{}

type Builder struct {
	lbMgr       *LBManager
	listenerMgr *ListenerManager
	sgMgr       *ServerGroupManager
}

type Applier struct {
	lbMgr       *LBManager
	listenerMgr *ListenerManager
	sgMgr       *ServerGroupManager
}

var _ driver.Driver = (*MockLB)(nil)

type MockLB struct {
	prvd    provider
	builder *Builder
	applier *Applier
	client  client.Client
}

func (e *MockLB) Apply(reqCtx *reqCtx.RequestContext) (map[string]*driver.Configuration, error) {
	return nil, nil
}

func (e *MockLB) Cleanup(reqCtx *reqCtx.RequestContext) (map[string]*driver.Configuration, error) {
	return nil, nil
}

func NewMockLoadBalancer(c *appconfig.CompletedConfig, mgr manager.Manager) (driver.Driver, error) {
	return &MockLB{prvd: NewProvider(""), client: mgr.GetClient()}, nil
}

func (e *MockLB) Service() predicate.Predicate {
	return predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			svc, ok := object.(*v1.Service)
			if !ok {
				return false
			}
			if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
				return false
			}
			if *svc.Spec.LoadBalancerClass != MockClass {
				return false
			}
			return true
		},
	)
}

func (e *MockLB) EndpointSlice() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			// TODO
			return false
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			// TODO
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
	}
}

func (e *MockLB) Node() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool { return false })
}

func (e *MockLB) NodePool() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool { return false })
}

func (e *MockLB) Init() error {
	lbMgr := &LBManager{}
	sgMgr := &ServerGroupManager{}
	lsMgr := &ListenerManager{}
	e.builder = &Builder{lbMgr: lbMgr, sgMgr: sgMgr, listenerMgr: lsMgr}
	e.applier = &Applier{lbMgr: lbMgr, sgMgr: sgMgr, listenerMgr: lsMgr}
	return nil
}
