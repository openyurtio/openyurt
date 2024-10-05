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

package util

import (
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
)

func IsNil(i interface{}) bool {
	if i == nil {
		return true
	}

	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Chan, reflect.Map:
		return reflect.ValueOf(i).IsNil()
	}
	return false
}

// Dropping `.metadata.managedFields` to improve memory usage
func TransformStripManagedFields() cache.TransformFunc {
	return func(in any) (any, error) {
		// Nilcheck managed fields to avoid hitting https://github.com/kubernetes/kubernetes/issues/124337
		if obj, err := meta.Accessor(in); err == nil && obj.GetManagedFields() != nil {
			obj.SetManagedFields(nil)
		}

		return in, nil
	}
}

const (
	// HttpHeaderContentType HTTP request header keyword: Content-Type which is used in HTTP request and response
	// headers to specify the media type of the entity body
	HttpHeaderContentType = "Content-Type"
	// HttpHeaderContentLength HTTP request header keyword: Content-Length which is used to indicate the size of the
	// message body, ensuring that the message can be transmitted and parsed correctly
	HttpHeaderContentLength = "Content-Length"
	// HttpHeaderTransferEncoding HTTP request header keyword: Transfer-Encoding which is used to indicate the HTTP
	// transmission encoding type used by the server
	HttpHeaderTransferEncoding = "Transfer-Encoding"

	// HttpContentTypeJson HTTP request Content-Type type: application/json which is used to indicate that the data
	// type transmitted in the HTTP request and response body is JSON
	HttpContentTypeJson = "application/json"
)
