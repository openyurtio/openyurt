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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// StatePending, StateSuccess and StateFailure are the Dragonfly manager job states.
	StatePending = "PENDING"
	StateSuccess = "SUCCESS"
	StateFailure = "FAILURE"

	jobsPath           = "/oapi/v1/jobs"
	defaultHTTPTimeout = 10 * time.Second
	maxErrorBodyBytes  = 512
)

// PreheatRequest describes an image preheat aimed at specific peers.
type PreheatRequest struct {
	// URL is the image manifest url, see ManifestURL.
	URL string
	// IPs are the peer ips to preheat on, at most 100.
	IPs []string
	// Username and Password authenticate against the image registry, optional.
	Username string
	Password string
}

// JobStatus is the state of a Dragonfly manager job.
type JobStatus struct {
	ID      int64
	State   string
	Message string
}

// Client talks to the Dragonfly manager open api.
type Client interface {
	CreatePreheatJob(ctx context.Context, token string, req PreheatRequest) (int64, error)
	GetJob(ctx context.Context, token string, id int64) (JobStatus, error)
}

type client struct {
	address string
	http    *http.Client
}

// NewClient returns a Client for the Dragonfly manager at address. A nil httpClient
// gets a default client with a 10s timeout.
func NewClient(address string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &client{address: strings.TrimRight(address, "/"), http: httpClient}
}

type preheatArgs struct {
	Type     string   `json:"type"`
	URL      string   `json:"url"`
	Scope    string   `json:"scope"`
	IPs      []string `json:"ips"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
}

type createJobRequest struct {
	Type string      `json:"type"`
	Args preheatArgs `json:"args"`
}

type jobResponse struct {
	ID     int64           `json:"id"`
	State  string          `json:"state"`
	Result json.RawMessage `json:"result"`
}

func (c *client) CreatePreheatJob(ctx context.Context, token string, req PreheatRequest) (int64, error) {
	body, err := json.Marshal(createJobRequest{
		Type: "preheat",
		Args: preheatArgs{
			Type:     "image",
			URL:      req.URL,
			Scope:    "all_peers",
			IPs:      req.IPs,
			Username: req.Username,
			Password: req.Password,
		},
	})
	if err != nil {
		return 0, err
	}

	var resp jobResponse
	if err := c.do(ctx, http.MethodPost, c.address+jobsPath, token, body, &resp); err != nil {
		return 0, fmt.Errorf("create dragonfly preheat job for %s: %w", req.URL, err)
	}
	return resp.ID, nil
}

func (c *client) GetJob(ctx context.Context, token string, id int64) (JobStatus, error) {
	var resp jobResponse
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s%s/%d", c.address, jobsPath, id), token, nil, &resp); err != nil {
		return JobStatus{}, fmt.Errorf("get dragonfly job %d: %w", id, err)
	}

	status := JobStatus{ID: resp.ID, State: resp.State}
	if resp.State == StateFailure && len(resp.Result) > 0 {
		var compact bytes.Buffer
		if json.Compact(&compact, resp.Result) == nil {
			status.Message = truncate(compact.String())
		}
	}
	return status, nil
}

func (c *client) do(ctx context.Context, method, url, token string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func truncate(s string) string {
	if len(s) <= maxErrorBodyBytes {
		return s
	}
	return s[:maxErrorBodyBytes] + "...(truncated)"
}
