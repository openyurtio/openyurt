package sharedcontext

import (
	"context"
	"sync"

	v1 "k8s.io/api/core/v1"
)

type RequestContext struct {
	Ctx     context.Context
	Service *v1.Service
	config  *GlobalConfig
}

type GlobalConfig struct {
	mu     sync.Mutex
	config map[any]any
}

func (g *GlobalConfig) Get(key any) any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.config[key]
}

func (g *GlobalConfig) Set(key, val any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config[key] = val
}

func (r *RequestContext) AnnotationGet(key string) string {
	if r.Service == nil || r.Service.Annotations == nil {
		return ""
	}
	return r.Service.Annotations[key]
}
