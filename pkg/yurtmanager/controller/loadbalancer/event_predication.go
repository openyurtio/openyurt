package loadbalancer

import (
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/driver"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = (*predicationForServiceEvent)(nil)

type predicationForServiceEvent struct {
	client  client.Client
	drivers driver.Drivers
}

func (p *predicationForServiceEvent) Create(evt event.CreateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Service().Create(evt)
	}
	return enqueue
}

func (p *predicationForServiceEvent) Delete(evt event.DeleteEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Service().Delete(evt)
	}
	return enqueue
}

func (p *predicationForServiceEvent) Update(evt event.UpdateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Service().Update(evt)
	}
	return enqueue
}

func (p *predicationForServiceEvent) Generic(evt event.GenericEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Service().Generic(evt)
	}
	return enqueue
}

func NewPredictionForServiceEvent(client client.Client, drivers driver.Drivers) *predicationForServiceEvent {
	return &predicationForServiceEvent{client: client, drivers: drivers}
}

var _ predicate.Predicate = (*predicationForEndpointSliceEvent)(nil)

type predicationForEndpointSliceEvent struct {
	client  client.Client
	drivers driver.Drivers
}

func (p *predicationForEndpointSliceEvent) Create(evt event.CreateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.EndpointSlice().Create(evt)
		if enqueue {
			break
		}
	}
	return enqueue
}

func (p *predicationForEndpointSliceEvent) Delete(evt event.DeleteEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.EndpointSlice().Delete(evt)
	}
	return enqueue
}

func (p *predicationForEndpointSliceEvent) Update(evt event.UpdateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.EndpointSlice().Update(evt)
	}
	return enqueue
}

func (p *predicationForEndpointSliceEvent) Generic(evt event.GenericEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.EndpointSlice().Generic(evt)
	}
	return enqueue
}

func NewPredictionForEndpointSliceEvent(client client.Client, drivers driver.Drivers) *predicationForEndpointSliceEvent {
	return &predicationForEndpointSliceEvent{client: client, drivers: drivers}
}

var _ predicate.Predicate = (*predicationForNodeEvent)(nil)

type predicationForNodeEvent struct {
	client  client.Client
	drivers driver.Drivers
}

func (p *predicationForNodeEvent) Create(evt event.CreateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Node().Create(evt)
	}
	return enqueue
}

func (p *predicationForNodeEvent) Delete(evt event.DeleteEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Node().Delete(evt)
	}
	return enqueue
}

func (p *predicationForNodeEvent) Update(evt event.UpdateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Node().Update(evt)
	}
	return enqueue
}

func (p *predicationForNodeEvent) Generic(evt event.GenericEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.Node().Generic(evt)
	}
	return enqueue
}

func NewPredictionForNodeEvent(client client.Client, drivers driver.Drivers) *predicationForNodeEvent {
	return &predicationForNodeEvent{client: client, drivers: drivers}
}

var _ predicate.Predicate = (*predicationForNodePoolEvent)(nil)

type predicationForNodePoolEvent struct {
	client  client.Client
	drivers driver.Drivers
}

func (p *predicationForNodePoolEvent) Create(evt event.CreateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.NodePool().Create(evt)
	}
	return enqueue
}

func (p *predicationForNodePoolEvent) Delete(evt event.DeleteEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.NodePool().Delete(evt)
	}
	return enqueue
}

func (p *predicationForNodePoolEvent) Update(evt event.UpdateEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.NodePool().Update(evt)
	}
	return enqueue
}

func (p *predicationForNodePoolEvent) Generic(evt event.GenericEvent) bool {
	enqueue := false
	for _, drv := range p.drivers {
		enqueue = enqueue || drv.NodePool().Generic(evt)
	}
	return enqueue
}

func NewPredictionForNodePoolEvent(client client.Client, drivers driver.Drivers) *predicationForNodePoolEvent {
	return &predicationForNodePoolEvent{client: client, drivers: drivers}
}
