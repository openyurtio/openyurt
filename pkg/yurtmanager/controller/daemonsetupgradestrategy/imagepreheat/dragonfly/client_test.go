/*
Copyright 2026 The OpenYurt Authors.

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

package dragonfly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAddr = "http://dfm:8080"

func newMockedClient(t *testing.T) Client {
	t.Helper()
	hc := &http.Client{}
	httpmock.ActivateNonDefault(hc)
	t.Cleanup(httpmock.DeactivateAndReset)
	return NewClient(testAddr, hc)
}

func TestCreatePreheatJob_RequestShape(t *testing.T) {
	c := newMockedClient(t)
	var gotBody map[string]any
	httpmock.RegisterResponder(http.MethodPost, testAddr+"/oapi/v1/jobs", func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		b, _ := io.ReadAll(req.Body)
		require.NoError(t, json.Unmarshal(b, &gotBody))
		return httpmock.NewStringResponse(http.StatusOK, `{"id":42,"state":"PENDING"}`), nil
	})

	id, err := c.CreatePreheatJob(context.Background(), "tok", PreheatRequest{
		URL: "http://r:5000/v2/demo/big/manifests/v2",
		IPs: []string{"10.0.0.1", "10.0.0.2"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)

	assert.Equal(t, "preheat", gotBody["type"])
	args := gotBody["args"].(map[string]any)
	assert.Equal(t, "image", args["type"])
	assert.Equal(t, "all_peers", args["scope"])
	assert.Equal(t, "http://r:5000/v2/demo/big/manifests/v2", args["url"])
	assert.Equal(t, []any{"10.0.0.1", "10.0.0.2"}, args["ips"])
	_, hasUser := args["username"]
	_, hasPass := args["password"]
	assert.False(t, hasUser)
	assert.False(t, hasPass)
}

func TestCreatePreheatJob_Credentials(t *testing.T) {
	c := newMockedClient(t)
	var gotBody map[string]any
	httpmock.RegisterResponder(http.MethodPost, testAddr+"/oapi/v1/jobs", func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		require.NoError(t, json.Unmarshal(b, &gotBody))
		return httpmock.NewStringResponse(http.StatusOK, `{"id":1}`), nil
	})
	_, err := c.CreatePreheatJob(context.Background(), "tok", PreheatRequest{URL: "u", IPs: []string{"1.1.1.1"}, Username: "u1", Password: "p1"})
	require.NoError(t, err)
	args := gotBody["args"].(map[string]any)
	assert.Equal(t, "u1", args["username"])
	assert.Equal(t, "p1", args["password"])
}

func TestCreatePreheatJob_Non2xx(t *testing.T) {
	c := newMockedClient(t)
	httpmock.RegisterResponder(http.MethodPost, testAddr+"/oapi/v1/jobs",
		httpmock.NewStringResponder(http.StatusUnauthorized, "invalid token"))
	_, err := c.CreatePreheatJob(context.Background(), "tok", PreheatRequest{URL: "u", IPs: []string{"1.1.1.1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "invalid token")
}

func TestCreatePreheatJob_Non2xxBodyTruncated(t *testing.T) {
	c := newMockedClient(t)
	httpmock.RegisterResponder(http.MethodPost, testAddr+"/oapi/v1/jobs",
		httpmock.NewStringResponder(http.StatusInternalServerError, strings.Repeat("x", 2000)))
	_, err := c.CreatePreheatJob(context.Background(), "tok", PreheatRequest{URL: "u", IPs: []string{"1.1.1.1"}})
	require.Error(t, err)
	assert.Less(t, len(err.Error()), 700)
}

func TestGetJob_States(t *testing.T) {
	c := newMockedClient(t)
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/1",
		httpmock.NewStringResponder(http.StatusOK, `{"id":1,"state":"SUCCESS"}`))
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/2",
		httpmock.NewStringResponder(http.StatusOK, `{"id":2,"state":"PENDING"}`))
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/3",
		httpmock.NewStringResponder(http.StatusOK, `{"id":3,"state":"FAILURE","result":{"error":"manifest unknown"}}`))

	st, err := c.GetJob(context.Background(), "tok", 1)
	require.NoError(t, err)
	assert.Equal(t, JobStatus{ID: 1, State: StateSuccess}, st)

	st, err = c.GetJob(context.Background(), "tok", 2)
	require.NoError(t, err)
	assert.Equal(t, StatePending, st.State)

	st, err = c.GetJob(context.Background(), "tok", 3)
	require.NoError(t, err)
	assert.Equal(t, StateFailure, st.State)
	assert.Contains(t, st.Message, "manifest unknown")
}

func TestGetJob_SendsToken(t *testing.T) {
	c := newMockedClient(t)
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/9", func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
		return httpmock.NewStringResponse(http.StatusOK, `{"id":9,"state":"SUCCESS"}`), nil
	})
	_, err := c.GetJob(context.Background(), "tok", 9)
	require.NoError(t, err)
}

func TestGetJob_BadJSON(t *testing.T) {
	c := newMockedClient(t)
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/1",
		httpmock.NewStringResponder(http.StatusOK, `not json`))
	_, err := c.GetJob(context.Background(), "tok", 1)
	assert.Error(t, err)
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	hc := &http.Client{}
	httpmock.ActivateNonDefault(hc)
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder(http.MethodGet, testAddr+"/oapi/v1/jobs/1",
		httpmock.NewStringResponder(http.StatusOK, `{"id":1,"state":"SUCCESS"}`))
	_, err := NewClient(testAddr+"/", hc).GetJob(context.Background(), "tok", 1)
	assert.NoError(t, err)
}
