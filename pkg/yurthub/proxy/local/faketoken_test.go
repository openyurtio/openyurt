package local

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiserverserviceaccount "k8s.io/apiserver/pkg/authentication/serviceaccount"
)

func TestCreateSerializer_UnsupportedContentType(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Content-Type", "application/unsupported")
	gvr := schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "tokenreviews"}
	sm := serializer.NewSerializerManager()

	serializer := createSerializer(req, gvr, sm)
	if serializer != nil {
		t.Errorf("Expected nil serializer for unsupported content type")
	}
}

func TestCreateSerializer_InvalidGVR(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Content-Type", "application/json")
	gvr := schema.GroupVersionResource{Group: "invalid", Version: "v1", Resource: "invalid"}
	sm := serializer.NewSerializerManager()

	serializer := createSerializer(req, gvr, sm)
	if serializer != nil {
		t.Errorf("Expected nil serializer for invalid GVR")
	}
}

func TestWriteRequestDirectly(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	data := []byte("test data")
	n := int64(len(data))

	writeRequestDirectly(w, req, data, n)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, but got %d", http.StatusCreated, resp.StatusCode)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	if !bytes.Equal(body, data) {
		t.Errorf("Expected response body %s, but got %s", string(data), string(body))
	}
}

func TestGetFakeToken(t *testing.T) {
	cases := []struct {
		namespace string
		name      string
		wantErr   bool
	}{
		{"test-namespace", "test-name", false},
		{"", "", false}, // Test with empty namespace and name
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("namespace=%s,name=%s", c.namespace, c.name), func(t *testing.T) {
			tokenString, err := getFakeToken(c.namespace, c.name)
			if (err != nil) != c.wantErr {
				t.Errorf("getFakeToken() error = %v, wantErr %v", err, c.wantErr)
				return
			}

			// Parse the token
			token, parseErr := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte("openyurt"), nil
			})

			if parseErr != nil {
				t.Errorf("Failed to parse token: %v", parseErr)
				return
			}

			if claims, ok := token.Claims.(*jwt.StandardClaims); ok {
				if claims.Issuer != "openyurt" {
					t.Errorf("Expected issuer openyurt, got %s", claims.Issuer)
				}
				expectedSubject := apiserverserviceaccount.MakeUsername(c.namespace, c.name)
				if claims.Subject != expectedSubject {
					t.Errorf("Expected subject %s, got %s", expectedSubject, claims.Subject)
				}
				// Check expiration is roughly 1 minute from now
				if time.Unix(claims.ExpiresAt, 0).Sub(time.Now()) > time.Minute*1 {
					t.Errorf("Token expires too late")
				}
				if time.Unix(claims.ExpiresAt, 0).Sub(time.Now()) < time.Second*50 {
					t.Errorf("Token expires too soon")
				}
			} else {
				t.Errorf("Claims are not of type *jwt.StandardClaims")
			}
		})
	}
}
