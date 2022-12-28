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

package utils

import (
	"reflect"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

type Pkey struct {
}

func (p Pkey) Key() string {
	return "test"
}

func TestValidateKey(t *testing.T) {
	k := Pkey{}
	tests := []struct {
		name         string
		key          storage.Key
		validKeyType interface{}
		expect       error
	}{
		{
			"normal",
			k,
			k,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := ValidateKey(tt.key, tt.validKeyType)
				if !reflect.DeepEqual(get, tt.expect) {
					t.Errorf("\texpect %v, but get %v", tt.expect, get)
				}
			}
		})
	}
}

func TestValidateKV(t *testing.T) {
	k := Pkey{}
	tests := []struct {
		name          string
		key           storage.Key
		content       []byte
		valideKeyType interface{}
		expect        error
	}{
		{
			"normal",
			k,
			[]byte{
				1,
				2,
			},
			k,
			nil,
		},
		{
			"empty",
			k,
			[]byte{},
			k,
			storage.ErrKeyHasNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := ValidateKV(tt.key, tt.content, tt.valideKeyType)
				if !reflect.DeepEqual(get, tt.expect) {
					t.Errorf("\texpect %v, but get %v", tt.expect, get)
				}
			}
		})
	}
}
