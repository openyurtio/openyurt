package util

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTransportManager is a mock of transport.Interface
type MockTransportManager struct {
	mock.Mock
}

// Close implements transport.Interface.
func (m *MockTransportManager) Close(address string) {
	panic("unimplemented")
}

func (m *MockTransportManager) CurrentTransport() http.RoundTripper {
	args := m.Called()
	return args.Get(0).(http.RoundTripper)
}

func (m *MockTransportManager) BearerTransport() http.RoundTripper {
	args := m.Called()
	return args.Get(0).(http.RoundTripper)
}

func TestNewRemoteProxy(t *testing.T) {
	// Setup
	mockURL, _ := url.Parse("http://example.com")
	mockTransport := &http.Transport{}
	mockTransportManager := new(MockTransportManager)
	stopCh := make(<-chan struct{})

	// Define test cases
	tests := []struct {
		name           string
		setupMocks     func(m *MockTransportManager)
		expectedError  string
		expectNilProxy bool
	}{
		{
			name: "Valid transports",
			setupMocks: func(m *MockTransportManager) {
				m.On("CurrentTransport").Return(mockTransport)
				m.On("BearerTransport").Return(mockTransport)
			},
			expectedError:  "",
			expectNilProxy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks for this test
			tt.setupMocks(mockTransportManager)

			// Execute
			proxy, err := NewRemoteProxy(mockURL, nil, nil, mockTransportManager, stopCh)

			// Assert
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, proxy)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, proxy)
				// Further assertions on proxy can be added here
			}
		})
	}
}

func TestRemoteServer(t *testing.T) {
	tests := []struct {
		name            string
		remoteServerURL string
		expectedURL     string
	}{
		{
			name:            "Test with valid URL",
			remoteServerURL: "http://example.com",
			expectedURL:     "http://example.com",
		},
		{
			name:            "Test with HTTPS URL",
			remoteServerURL: "https://example.com",
			expectedURL:     "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remoteServer, _ := url.Parse(tt.remoteServerURL)
			proxy := &RemoteProxy{remoteServer: remoteServer}

			resultURL := proxy.RemoteServer()

			assert.Equal(t, tt.expectedURL, resultURL.String(), "The returned URL should match the expected URL")
		})
	}
}
func TestRemoteProxy_Name(t *testing.T) {
	mockURL, _ := url.Parse("http://example.com")
	proxy := &RemoteProxy{
		remoteServer: mockURL,
	}

	expectedName := "http://example.com"
	actualName := proxy.Name()

	assert.Equal(t, expectedName, actualName, "The returned name should match the expected name")
}
func TestRemoteProxy_ServeHTTP(t *testing.T) {
	mockRemoteServer, _ := url.Parse("http://example.com")
	mockTransport := &http.Transport{}
	mockTransportManager := new(MockTransportManager)
	mockTransportManager.On("CurrentTransport").Return(mockTransport)
	mockTransportManager.On("BearerTransport").Return(mockTransport)
	stopCh := make(<-chan struct{})

	proxy, err := NewRemoteProxy(mockRemoteServer, nil, nil, mockTransportManager, stopCh)
	if err != nil {
		t.Fatalf("Failed to create RemoteProxy: %v", err)
	}

	tests := []struct {
		name          string
		request       *http.Request
		expectBearer  bool
		expectUpgrade bool
	}{
		{
			name:          "Non-upgrade request",
			request:       httptest.NewRequest("GET", "http://example.com", nil),
			expectBearer:  false,
			expectUpgrade: false,
		},
		{
			name:          "Upgrade request with bearer token",
			request:       httptest.NewRequest("GET", "http://example.com", nil),
			expectBearer:  true,
			expectUpgrade: true,
		},
		{
			name:          "Upgrade request without bearer token",
			request:       httptest.NewRequest("GET", "http://example.com", nil),
			expectBearer:  false,
			expectUpgrade: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectUpgrade {
				tt.request.Header.Set("Connection", "Upgrade")
				tt.request.Header.Set("Upgrade", "websocket")
			}
			if tt.expectBearer {
				tt.request.Header.Set("Authorization", "Bearer some-token")
			}

			rw := httptest.NewRecorder()

			proxy.ServeHTTP(rw, tt.request)

			// Assertions for each scenario should be implemented here.
			// Since we're mocking external dependencies, we'd need to assert based on the behavior of our mock objects.
			// This could involve checking if the response writer was written to, or if certain headers were set.
			// However, without a concrete way to mock the internal handlers, detailed assertions are limited.
		})
	}
}

func TestIsBearerRequest(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedResult bool
	}{
		{
			name:           "Valid bearer token",
			authHeader:     "Bearer abc123",
			expectedResult: true,
		},
		{
			name:           "Invalid bearer token with wrong prefix",
			authHeader:     "Bear abc123",
			expectedResult: false,
		},
		{
			name:           "Invalid bearer token with extra parts",
			authHeader:     "Bearer abc123 extra",
			expectedResult: false,
		},
		{
			name:           "Empty Authorization header",
			authHeader:     "",
			expectedResult: false,
		},
		{
			name:           "Non-bearer Authorization header",
			authHeader:     "Basic YWxhZGRpbjpvcGVuc2VzYW1l",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			req.Header.Set("Authorization", tt.authHeader)
			result := isBearerRequest(req)
			assert.Equal(t, tt.expectedResult, result, "isBearerRequest should return the expected result")
		})
	}
}
