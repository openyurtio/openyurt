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

package jwt

import (
	"testing"

	"github.com/go-jose/go-jose/v3/jwt"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
)

func TestJwt(t *testing.T) {

	bearerToken := "eyJhbGciOiJSUzI1NiIsImtpZCI6InVfTVZpZWIySUFUTzQ4NjlkM0VwTlBRb0xJOWVKUGg1ZXVzbEdaY0ZxckEifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJrdWJlLXN5c3RlbSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6a3ViZS1zeXN0ZW06ZGVmYXVsdCJ9.sFpHHg4o88Z0CBJseMBvBeP00bS5isLBmQJpAOiYs3BTkEAD63YLTnDURt0r3I9QjtcP0DZAb5wSOccGChMAFVtxMIoIoZC6Mk4FSB720kawRxFVujNFR1T7uVV_dbpEU-wsxSb9-Y4ILVknuJR9t35x6lUbRkUE9tN1wDy4DH296C3gEGNJf8sbJMERZzOckc82_BamlCzaieo1nX396KafxdQGVIgxstx88hm_rgpjDy3LA1GNsx6x2pqXdzZ8mufQt7sTljRorXUk-rNU6y9wX2RvIMO8tNiPClNkdIpgpmeQo-g7XZivpEeq3VzoeExphRbusgCtO9T9tgU64w"
	var bearerClaims = jwt.Claims{}

	if token, err := jwt.ParseSigned(bearerToken); err == nil {
		if err := token.UnsafeClaimsWithoutVerification(&bearerClaims); err == nil {

			if tenantNs, username, err := serviceaccount.SplitUsername(bearerClaims.Subject); err == nil {
				t.Logf("succeed to parse token, ns: %s, username: %s", tenantNs, username)
			} else {
				t.Errorf("failed to parse jwt token, %v", err)
			}
		} else {
			t.Errorf("failed to parse jwt token, %v", err)
		}
	} else {
		t.Errorf("failed to parse jwt token, %v", err)
	}
}
