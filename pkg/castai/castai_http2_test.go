package castai

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"castai-agent/internal/config"
)

// transientErrorTransport simulates a transient network error on first request
// then allows subsequent requests to pass through
type transientErrorTransport struct {
	transport     *http.Transport
	attemptNumber int
	logger        func(string, ...interface{})
}

func (s *transientErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.attemptNumber++
	if s.logger != nil {
		s.logger("transientErrorTransport: attempt %d", s.attemptNumber)
	}

	// First attempt: return a transient error
	if s.attemptNumber == 1 {
		if s.logger != nil {
			s.logger("transientErrorTransport: returning transient error on attempt 1")
		}
		return nil, fmt.Errorf("connection reset by peer")
	}

	// Subsequent attempts: pass through to real transport
	return s.transport.RoundTrip(req)
}

// TestSendDelta_HTTP2Success verifies large payloads work over HTTP/2
func TestSendDelta_HTTP2Success(t *testing.T) {
	t.Cleanup(func() { config.Reset() })

	var receivedBytes int

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		require.Greater(t, r.ContentLength, int64(0), "ContentLength should be set")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Logf("server read error: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}
		receivedBytes = len(body)
		t.Logf("server received %d bytes", receivedBytes)

		require.Equal(t, "gzip", r.Header.Get("Content-Encoding"))

		gr, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err)
		defer gr.Close()

		var delta Delta
		require.NoError(t, json.NewDecoder(gr).Decode(&delta))

		w.WriteHeader(http.StatusNoContent)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	serverURL := strings.TrimPrefix(ts.URL, "https://")
	t.Setenv("API_URL", serverURL)
	t.Setenv("API_KEY", "test-key")

	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	_, err := http2.ConfigureTransports(transport)
	require.NoError(t, err)

	httpClient := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: transport,
	}

	delta := generateLargeDelta(t, 50000)
	client := NewClient(logrus.New(), nil, httpClient)

	err = client.SendDelta(context.Background(), "test-cluster", delta)
	require.NoError(t, err, "large delta should succeed with well-behaved server")

	require.Greater(t, receivedBytes, 1024*1024, "should receive at least 1MB of data")
	t.Logf("successfully received %d bytes", receivedBytes)
}

// TestSendDelta_HTTP5xx_NotRetried shows that the client does NOT automatically retry
// on HTTP 5xx errors - it returns DeltaRequestError immediately.
func TestSendDelta_HTTP5xx_NotRetried(t *testing.T) {
	t.Cleanup(func() { config.Reset() })

	attempt := 0

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		t.Logf("attempt %d", attempt)

		body, _ := io.ReadAll(r.Body)
		t.Logf("attempt %d body size: %d", attempt, len(body))

		if attempt == 1 {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	serverURL := strings.TrimPrefix(ts.URL, "https://")
	t.Setenv("API_URL", serverURL)
	t.Setenv("API_KEY", "test-key")

	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	_, err := http2.ConfigureTransports(transport)
	require.NoError(t, err)

	httpClient := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: transport,
	}

	delta := generateLargeDelta(t, 5000)
	client := NewClient(logrus.New(), nil, httpClient)

	err = client.SendDelta(context.Background(), "test-cluster", delta)

	t.Logf("error: %v", err)
	t.Logf("attempts: %d", attempt)

	require.Equal(t, 1, attempt, "should NOT retry on HTTP 5xx")
	require.Contains(t, err.Error(), "500", "should be 500 Internal Server Error")
}

// TestSendDelta_RetrySucceeds_AfterTransientNetworkError verifies that when the first
// request fails with a transient network error (before body is read), the retry succeeds.
func TestSendDelta_RetrySucceeds_AfterTransientNetworkError(t *testing.T) {
	t.Cleanup(func() { config.Reset() })

	serverAttempt := 0

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverAttempt++
		t.Logf("server: attempt %d", serverAttempt)

		body, _ := io.ReadAll(r.Body)
		t.Logf("server: attempt %d received %d bytes", serverAttempt, len(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	serverURL := strings.TrimPrefix(ts.URL, "https://")
	t.Setenv("API_URL", serverURL)
	t.Setenv("API_KEY", "test-key")

	baseTransport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	_, err := http2.ConfigureTransports(baseTransport)
	require.NoError(t, err)

	// Use custom transport that fails first request, succeeds on retry
	customTransport := &transientErrorTransport{
		transport: baseTransport,
		logger:    t.Logf,
	}

	httpClient := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: customTransport,
	}

	delta := generateLargeDelta(t, 5000)
	client := NewClient(logrus.New(), nil, httpClient)

	err = client.SendDelta(context.Background(), "test-cluster", delta)

	require.NoError(t, err, "retry should succeed after transient error at transport level")
	require.Equal(t, 1, serverAttempt, "server receives only the retry (first failed at transport)")
}

// TestSendDelta_RetrySucceeds_AfterBodyConsumed is the primary regression test.
// It verifies that when the body is fully consumed on attempt 1 (and a network error
// is returned), the retry on attempt 2 can still send a valid body.
// This FAILS with io.Pipe (body exhausted after attempt 1), PASSES with bytes.Buffer+GetBody.
func TestSendDelta_RetrySucceeds_AfterBodyConsumed(t *testing.T) {
	t.Cleanup(func() { config.Reset() })

	serverAttempt := 0
	var receivedItemCount int

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverAttempt++
		t.Logf("server: attempt %d", serverAttempt)

		require.Greater(t, r.ContentLength, int64(0), "ContentLength should be set")

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		gr, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err)
		defer gr.Close()

		var delta Delta
		require.NoError(t, json.NewDecoder(gr).Decode(&delta))
		receivedItemCount = len(delta.Items)
		t.Logf("server: received %d items", receivedItemCount)

		w.WriteHeader(http.StatusNoContent)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	serverURL := strings.TrimPrefix(ts.URL, "https://")
	t.Setenv("API_URL", serverURL)
	t.Setenv("API_KEY", "test-key")

	baseTransport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	_, err := http2.ConfigureTransports(baseTransport)
	require.NoError(t, err)

	// bodyConsumingTransport drains the body on attempt 1, then returns a network error.
	// On attempt 2, it forwards to the real transport.
	// With io.Pipe: attempt 2 sends an empty body → server fails.
	// With bytes.Buffer+GetBody: attempt 2 replays the full body → server succeeds.
	type bodyConsumingTransport struct {
		transport     *http.Transport
		attemptNumber int
	}
	bct := &bodyConsumingTransport{transport: baseTransport}

	httpClient := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			bct.attemptNumber++
			t.Logf("transport: attempt %d", bct.attemptNumber)

			if bct.attemptNumber == 1 {
				// Fully drain the body, then return a network error
				if req.Body != nil {
					_, _ = io.ReadAll(req.Body)
					_ = req.Body.Close()
				}
				return nil, fmt.Errorf("connection reset by peer")
			}
			return bct.transport.RoundTrip(req)
		}),
	}

	delta := generateLargeDelta(t, 1000)
	c := NewClient(logrus.New(), nil, httpClient)

	err = c.SendDelta(context.Background(), "test-cluster", delta)
	require.NoError(t, err, "retry should succeed after body was consumed on first attempt")
	require.Equal(t, 1, serverAttempt, "server should receive exactly one successful request")
	require.Equal(t, 1000, receivedItemCount, "server should receive all 1000 items on retry")
}

// roundTripFunc allows using a function as an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func generateLargeDelta(t *testing.T, itemCount int) *Delta {
	items := make([]*DeltaItem, itemCount)

	padding := strings.Repeat("x", 500)
	padding2 := strings.Repeat("y", 400)

	for i := 0; i < itemCount; i++ {
		data := json.RawMessage(fmt.Sprintf(
			`{"metadata":{"name":"pod-%d","namespace":"default","uid":"%08x-%04x-%04x-%04x-%012x","resourceVersion":"12345","creationTimestamp":"2024-01-01T00:00:00Z"},"spec":{"containers":[{"name":"app","image":"nginx:latest","resources":{"requests":{"cpu":"100m","memory":"128Mi"}}}]},"status":{"phase":"Running"},"padding":"%s","padding2":"%s"}`,
			i,
			time.Now().UnixNano()&0xFFFFFFFF,
			time.Now().Nanosecond()&0xFFFF,
			time.Now().Nanosecond()&0xFFFF,
			time.Now().Nanosecond()&0xFFFF,
			time.Now().UnixNano()&0xFFFFFFFFFFFF,
			padding,
			padding2,
		))

		items[i] = &DeltaItem{
			Event:     EventAdd,
			Kind:      "Pod",
			Data:      &data,
			CreatedAt: time.Now().UTC(),
		}
	}

	return &Delta{
		ClusterID:      "test-cluster-id",
		ClusterVersion: "1.28",
		FullSnapshot:   true,
		Items:          items,
	}
}
