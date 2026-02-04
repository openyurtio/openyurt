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

package local

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiserverserviceaccount "k8s.io/apiserver/pkg/authentication/serviceaccount"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

func TestWithFakeTokenInjectNilRequestInfo(t *testing.T) {
	t.Run("nil request info delegates to inner handler", func(t *testing.T) {
		innerCalled := false
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		sm := serializer.NewSerializerManager()
		wrapped := WithFakeTokenInject(innerHandler, sm)

		req, _ := http.NewRequest("POST", "/api/v1/namespaces/default/serviceaccounts/test/token", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		assert.True(t, innerCalled, "inner handler should be called when RequestInfo is nil")
		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}

func TestCreateSerializer(t *testing.T) {
	tests := []struct {
		name             string
		contentType      string
		gvr              schema.GroupVersionResource
		expectSerializer bool
	}{
		{
			name:             "should return nil serializer when content type is unsupported",
			contentType:      "application/unsupported",
			gvr:              schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "tokenreviews"},
			expectSerializer: false,
		},
		{
			name:             "should return nil serializer when GVR is invalid",
			contentType:      "application/json",
			gvr:              schema.GroupVersionResource{Group: "invalid", Version: "v1", Resource: "invalid"},
			expectSerializer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			req.Header.Set("Content-Type", tt.contentType)
			sm := serializer.NewSerializerManager()

			serializer := createSerializer(req, tt.gvr, sm)
			if tt.expectSerializer {
				assert.NotNil(t, serializer, "Expected serializer to be created")
			} else {
				assert.Nil(t, serializer, "Expected nil serializer for unsupported content type or invalid GVR")
			}
		})
	}
}

func TestWriteRequestDirectly(t *testing.T) {
	t.Run("should write request directly when called with valid data", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		data := []byte("test data")
		n := int64(len(data))

		writeRequestDirectly(w, req, data, n)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code %d, but got %d", http.StatusCreated, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, data, body, "Expected response body %s, but got %s", string(data), string(body))
	})
}

func TestGetFakeToken(t *testing.T) {
	cases := []struct {
		name      string
		namespace string
		nameArg   string
		wantErr   bool
	}{
		{
			name:      "should return a valid token when namespace and name are provided",
			namespace: "test-namespace",
			nameArg:   "test-name",
			wantErr:   false,
		},
		{
			name:      "should return a valid token when namespace and name are empty",
			namespace: "",
			nameArg:   "",
			wantErr:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokenString, err := getFakeToken(c.namespace, c.nameArg)
			if c.wantErr {
				assert.Error(t, err, "Expected error but got none")
				return
			}

			assert.NoError(t, err, "Expected no error but got: %v", err)

			token, parseErr := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte("openyurt"), nil
			})
			assert.NoError(t, parseErr, "Failed to parse token: %v", parseErr)

			if claims, ok := token.Claims.(*jwt.StandardClaims); assert.True(t, ok, "Claims are not of type *jwt.StandardClaims") {
				assert.Equal(t, "openyurt", claims.Issuer, "Expected issuer openyurt, got %s", claims.Issuer)
				expectedSubject := apiserverserviceaccount.MakeUsername(c.namespace, c.nameArg)
				assert.Equal(t, expectedSubject, claims.Subject, "Expected subject %s, got %s", expectedSubject, claims.Subject)

				expiration := time.Unix(claims.ExpiresAt, 0)
				assert.WithinDuration(t, time.Now().Add(1*time.Minute), expiration, time.Second*10, "Token expiration time is incorrect")
			}
		})
	}
}
