/*
Copyright 2022 The OpenYurt Authors.

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

package otaupdate

import (
	"net/http"

	"k8s.io/klog/v2"
)

// returnErr write the given error to response and set response header to the given error type
func returnErr(err error, w http.ResponseWriter,errType int) {
	w.WriteHeader(errType)
	n := len([]byte(err.Error()))
	nw, e := w.Write([]byte(err.Error()))
	if e != nil || nw != n {
		klog.Errorf("write resp for request, expect %d bytes but write %d bytes with error, %v", n, nw, e)
	}
}
