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

package utils

import "sync"

type Option interface {
	SetProxyOption(enable bool)
	SetTunnelOption(enable bool)
	GetProxyOption() bool
	GetTunnelOption() bool
	Reset()
}

type ServerOption struct {
	mu           sync.RWMutex
	enableProxy  bool
	enableTunnel bool
}

func NewOption() Option {
	return &ServerOption{enableTunnel: false, enableProxy: false}
}

func (o *ServerOption) SetProxyOption(enable bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enableProxy = enable
}

func (o *ServerOption) SetTunnelOption(enable bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enableTunnel = enable
}

func (o *ServerOption) GetProxyOption() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.enableProxy
}

func (o *ServerOption) GetTunnelOption() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.enableTunnel
}

func (o *ServerOption) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enableTunnel = false
	o.enableProxy = false
}
