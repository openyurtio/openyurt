/*
Copyright 2021 The OpenYurt Authors.

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

package filter

import (
	"io"
	"net/http"

	apirequest "k8s.io/apiserver/pkg/endpoints/request"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type filterChain []Interface

func (fc filterChain) Approve(comp, resource, verb string) bool {
	for _, f := range fc {
		if f.Approve(comp, resource, verb) {
			return true
		}
	}

	return false
}

func (fc filterChain) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok {
		return 0, rc, nil
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok {
		return 0, rc, nil
	}

	for _, f := range fc {
		if !f.Approve(comp, info.Resource, info.Verb) {
			continue
		}

		return f.Filter(req, rc, stopCh)
	}

	return 0, rc, nil
}
