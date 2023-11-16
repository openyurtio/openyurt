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
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiserverserviceaccount "k8s.io/apiserver/pkg/authentication/serviceaccount"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func WithFakeTokenInject(handler http.Handler, serializerManager *serializer.SerializerManager) http.Handler {
	tokenRequestGVR := authv1.SchemeGroupVersion.WithResource("tokenrequests")
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		info, _ := apirequest.RequestInfoFrom(ctx)
		if info.Resource == "serviceaccounts" && info.Subresource == "token" {
			klog.Infof("find serviceaccounts token request when cluster is unhealthy, try to write fake token to response.")
			var buf bytes.Buffer
			headerNStr := req.Header.Get(yurtutil.HttpHeaderContentLength)
			headerN, _ := strconv.Atoi(headerNStr)
			n, err := buf.ReadFrom(req.Body)
			if err != nil || (headerN != 0 && int(n) != headerN) {
				klog.Warningf("read body of post request when cluster is unhealthy, expect %d bytes but get %d bytes with error, %v", headerN, n, err)
			}

			s := createSerializer(req, tokenRequestGVR, serializerManager)
			if s == nil {
				klog.Errorf("skip fake token inject for request %s when cluster is unhealthy, could not create serializer.", util.ReqString(req))
				writeRequestDirectly(w, req, buf.Bytes(), n)
				return
			}

			tokenRequset, err := getTokenRequestWithFakeToken(buf.Bytes(), info, req, s)
			if err != nil {
				klog.Errorf("skip fake token inject for request %s when cluster is unhealthy, could not get token request: %v", util.ReqString(req), err)
				writeRequestDirectly(w, req, buf.Bytes(), n)
				return
			}

			klog.Infof("write fake token for request %s when cluster is unhealthy", util.ReqString(req))
			err = util.WriteObject(http.StatusCreated, tokenRequset, w, req)
			if err != nil {
				klog.Errorf("write fake token resp for token request when cluster is unhealthy with error, %v", err)
			}
			return
		}

		handler.ServeHTTP(w, req)
	})
}

func createSerializer(req *http.Request, gvr schema.GroupVersionResource, sm *serializer.SerializerManager) *serializer.Serializer {
	ctx := req.Context()
	reqContentType, _ := util.ReqContentTypeFrom(ctx)
	respContentType := reqContentType
	return sm.CreateSerializer(respContentType, gvr.Group, gvr.Version, gvr.Resource)
}

func writeRequestDirectly(w http.ResponseWriter, req *http.Request, data []byte, n int64) {
	copyHeader(w.Header(), req.Header)
	w.WriteHeader(http.StatusCreated)
	nw, err := w.Write(data)
	if err != nil || nw != int(n) {
		klog.Errorf("write resp for token request when cluster is unhealthy, expect %d bytes but write %d bytes with error, %v", n, nw, err)
	}
}

func getTokenRequestWithFakeToken(data []byte, info *apirequest.RequestInfo, req *http.Request, s *serializer.Serializer) (*authv1.TokenRequest, error) {
	obj, err := s.Decode(data)
	if err != nil || obj == nil {
		return nil, errors.Errorf("decode reuqest with error %v", err)
	}
	if tokenRequest, ok := obj.(*authv1.TokenRequest); ok {
		token, err := getFakeToken(info.Namespace, info.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "get fake token")
		}
		klog.V(5).Infof("the fake token of request %s is %s", util.ReqString(req), token)
		tokenRequest.Status = authv1.TokenRequestStatus{
			Token:               token,
			ExpirationTimestamp: metav1.NewTime(time.Now().Add(1 * time.Minute)),
		}
		return tokenRequest, nil
	}

	return nil, errors.Errorf("trans request object to token request failed.")
}

func getFakeToken(namespace, name string) (string, error) {
	expireTime := time.Now().Add(1 * time.Minute)
	claims := &jwt.StandardClaims{
		ExpiresAt: expireTime.Unix(),
		IssuedAt:  time.Now().Unix(),
		Issuer:    "openyurt",
		Subject:   apiserverserviceaccount.MakeUsername(namespace, name),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("openyurt"))
}
