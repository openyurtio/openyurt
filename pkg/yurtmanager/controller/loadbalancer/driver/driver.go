package driver

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/driver/mock"
)

type ActionType string

const (
	Create = ActionType("Create")
	Update = ActionType("Update")
	Delete = ActionType("Delete")
)

type AccessPoint struct {
	Name    string
	Address string
}

type Annotations interface {
	Get(key string) (value string)
	Has(key string) (exists bool)
}

type Set map[string]string

// Has returns whether the provided label exists in the map.
func (ls Set) Has(label string) bool {
	_, exists := ls[label]
	return exists
}

// Get returns the value in the map for the provided label.
func (ls Set) Get(label string) string {
	return ls[label]
}

type Configuration struct {
	PoolName     string
	Action       ActionType
	AccessPoints []AccessPoint
	ErrorInfo    error
	Config       map[string]string
}

type LoadBalancer interface {
	ID() string
	IsMatch(svc *v1.Service) bool
	Backends() []*v1.Node
	AccessPoints() []AccessPoint
	Extra() map[string]string
}

// Service and EndpointSlice
type Predication interface {
	Service() predicate.Predicate
	EndpointSlice() predicate.Predicate
	Node() predicate.Predicate
	NodePool() predicate.Predicate
}

type Driver interface {
	Predication
	Init() error
	List(ctx context.Context, svc *v1.Service) ([]LoadBalancer, error)
	Filter(ctx context.Context, svc *v1.Service, nodes []*v1.Node) ([]*v1.Node, error)
	Get(ctx context.Context, svc *v1.Service, nodes []*v1.Node) (LoadBalancer, error)
	Create(ctx context.Context, svc *v1.Service, nodes []*v1.Node) (LoadBalancer, error)
	Update(ctx context.Context, svc *v1.Service, nodes []*v1.Node, lb LoadBalancer) (LoadBalancer, error)
	Delete(ctx context.Context, svc *v1.Service, lb LoadBalancer) (LoadBalancer, error)
}

type Drivers map[string]Driver

func NewDrivers(cfg *appconfig.CompletedConfig, mgr manager.Manager) (Drivers, error) {
	drivers := make(map[string]Driver, 0)
	err := Register(drivers, cfg, mgr, mock.MockClass, mock.NewMockLoadBalancer)
	if err != nil {
		return nil, err
	}
	return drivers, nil
}

type NewDriverFunc func(cfg *appconfig.CompletedConfig, mgr manager.Manager) (Driver, error)

func Register(drivers Drivers, cfg *appconfig.CompletedConfig, mgr manager.Manager, name string, fn NewDriverFunc) error {
	if cfg == nil {
		return fmt.Errorf("config is empty")
	}
	drv, err := fn(cfg, mgr)
	if err != nil {
		return fmt.Errorf("can not new driver %s, error %s", name, err.Error())
	}
	drivers[name] = drv
	return nil
}
