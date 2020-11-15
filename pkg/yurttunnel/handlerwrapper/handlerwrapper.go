/*
Copyright 2020 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlerwrapper

import (
	"net/http"

	"k8s.io/klog/v2"
)

// Middleware takes in one Handler and wrap it within another
type Middleware interface {
	WrapHandler(http.Handler) http.Handler
	Name() string
}

var Middlewares []Middleware

// Register an middleware
func Register(m Middleware) {
	klog.V(4).Infof("register middleware: %s", m.Name())
	Middlewares = append(Middlewares, m)
}
