package castai

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"castai-agent/internal/config"
)

// http2ProtocolErrorServer resets the first stream with RST_STREAM PROTOCOL_ERROR,
// then responds 200 OK to all subsequent streams.
type http2ProtocolErrorServer struct {
	addr       string
	listener   net.Listener
	resetsSent atomic.Int64
	okSent     atomic.Int64
}

func newHTTP2ProtocolErrorServer(t *testing.T) *http2ProtocolErrorServer {
	t.Helper()
	cert, err := generateTestCert()
	require.NoError(t, err)

	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2"},
	})
	require.NoError(t, err)

	s := &http2ProtocolErrorServer{addr: l.Addr().String(), listener: l}
	go s.serve()
	t.Cleanup(func() { _ = l.Close() })
	return s
}

func (s *http2ProtocolErrorServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *http2ProtocolErrorServer) handleConn(conn net.Conn) {
	defer conn.Close()

	preface := make([]byte, 24)
	if _, err := io.ReadFull(conn, preface); err != nil {
		return
	}

	framer := http2.NewFramer(conn, conn)
	framer.AllowIllegalWrites = true

	if err := framer.WriteSettings(); err != nil {
		return
	}

	for {
		f, err := framer.ReadFrame()
		if err != nil {
			return
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				if err := framer.WriteSettingsAck(); err != nil {
					return
				}
			}
		case *http2.WindowUpdateFrame:
		case *http2.PingFrame:
			if !f.IsAck() {
				if err := framer.WritePing(true, f.Data); err != nil {
					return
				}
			}
		case *http2.HeadersFrame:
			if f.StreamEnded() {
				s.respondOrReset(framer, f.StreamID)
			}
		case *http2.DataFrame:
			_, _ = io.ReadAll(bytes.NewReader(f.Data()))
			if f.StreamEnded() {
				s.respondOrReset(framer, f.StreamID)
			}
		case *http2.RSTStreamFrame, *http2.GoAwayFrame:
			return
		}
	}
}

func (s *http2ProtocolErrorServer) respondOrReset(framer *http2.Framer, streamID uint32) {
	if s.resetsSent.CompareAndSwap(0, 1) {
		_ = framer.WriteRSTStream(streamID, http2.ErrCodeProtocol)
		return
	}
	s.okSent.Add(1)
	var hbuf bytes.Buffer
	enc := hpack.NewEncoder(&hbuf)
	enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hbuf.Bytes(),
		EndHeaders:    true,
	})
	_ = framer.WriteData(streamID, true, []byte(`{}`))
}

func generateTestCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
}

func newHTTP2TestClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		},
	}
}

func newSendDeltaClient(t *testing.T, httpClient *http.Client, apiURL string) Client {
	t.Helper()
	require.NoError(t, os.Setenv("API_KEY", "test-key"))
	require.NoError(t, os.Setenv("API_URL", apiURL))
	return NewClient(logrus.New(), nil, httpClient)
}

func testDelta() *Delta {
	data := json.RawMessage(`"payload"`)
	return &Delta{
		ClusterID:      "test-cluster",
		ClusterVersion: "1.29",
		FullSnapshot:   true,
		Items: []*DeltaItem{
			{Event: EventAdd, Kind: "Pod", Data: &data, CreatedAt: time.Now().UTC()},
		},
	}
}

func TestSendDelta_GetBody_RetriesAfterProtocolError(t *testing.T) {
	t.Cleanup(config.Reset)
	t.Cleanup(os.Clearenv)

	srv := newHTTP2ProtocolErrorServer(t)
	httpClient := newHTTP2TestClient()
	c := newSendDeltaClient(t, httpClient, "https://"+srv.addr)

	err := c.SendDelta(context.Background(), "test-cluster", testDelta())

	require.NoError(t, err)
	require.EqualValues(t, 1, srv.resetsSent.Load(), "server should have sent exactly one RST_STREAM")
	require.EqualValues(t, 1, srv.okSent.Load(), "server should have responded OK to the retry")
}

func TestSendDelta_GetBody_DecodesValidPayloadAfterRetry(t *testing.T) {
	t.Cleanup(config.Reset)
	t.Cleanup(os.Clearenv)

	d := testDelta()

	var capturedBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			capturedBody = body
		}
		w.WriteHeader(http.StatusOK)
	})

	cert, err := generateTestCert()
	require.NoError(t, err)
	stdSrv := &http.Server{
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	require.NoError(t, http2.ConfigureServer(stdSrv, nil))

	l, err := tls.Listen("tcp", "127.0.0.1:0", stdSrv.TLSConfig)
	require.NoError(t, err)
	go stdSrv.Serve(l) //nolint:errcheck
	t.Cleanup(func() { _ = stdSrv.Close() })

	require.NoError(t, os.Setenv("API_KEY", "test-key"))
	require.NoError(t, os.Setenv("API_URL", "https://"+l.Addr().String()))
	c := NewClient(logrus.New(), nil, newHTTP2TestClient())

	err = c.SendDelta(context.Background(), d.ClusterID, d)
	require.NoError(t, err)
	require.NotEmpty(t, capturedBody, "server received no body")

	zr, err := gzip.NewReader(bytes.NewReader(capturedBody))
	require.NoError(t, err)
	defer zr.Close()

	got := &Delta{}
	require.NoError(t, json.NewDecoder(zr).Decode(got))
	require.Equal(t, d.ClusterID, got.ClusterID)
	require.Equal(t, d.FullSnapshot, got.FullSnapshot)
	require.Len(t, got.Items, len(d.Items))
}

func TestSendDelta_GetBody_PipeIsNotReused(t *testing.T) {
	t.Cleanup(config.Reset)
	t.Cleanup(os.Clearenv)
	require.NoError(t, os.Setenv("API_KEY", "test-key"))
	require.NoError(t, os.Setenv("API_URL", "https://example.com"))

	d := testDelta()
	log := logrus.New()
	log.SetOutput(io.Discard)

	newDeltaPipe := func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			defer func() {
				if err := pw.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					t.Logf("pipe close error: %v", err)
				}
			}()
			gz := gzip.NewWriter(pw)
			defer gz.Close() //nolint:errcheck
			_ = json.NewEncoder(gz).Encode(d)
		}()
		return pr, nil
	}

	decode := func(rc io.ReadCloser) *Delta {
		defer rc.Close()
		zr, err := gzip.NewReader(rc)
		require.NoError(t, err)
		defer zr.Close()
		got := &Delta{}
		require.NoError(t, json.NewDecoder(zr).Decode(got))
		return got
	}

	r1, err := newDeltaPipe()
	require.NoError(t, err)
	r2, err := newDeltaPipe()
	require.NoError(t, err)

	got1 := decode(r1)
	got2 := decode(r2)

	require.Equal(t, d.ClusterID, got1.ClusterID)
	require.Equal(t, d.ClusterID, got2.ClusterID)
	require.Equal(t, got1, got2, "both pipes should produce identical deltas")
}

func TestLogHTTP2RequestError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantFields   logrus.Fields
		wantNoFields []string
	}{
		{
			name: "stream error without cause",
			err:  http2.StreamError{StreamID: 1, Code: http2.ErrCodeProtocol},
			wantFields: logrus.Fields{
				"http2_stream_id":  uint32(1),
				"http2_error_code": http2.ErrCodeProtocol,
			},
			wantNoFields: []string{"http2_cause"},
		},
		{
			name: "stream error with cause",
			err:  http2.StreamError{StreamID: 1, Code: http2.ErrCodeProtocol, Cause: errors.New("received from peer")},
			wantFields: logrus.Fields{
				"http2_stream_id":  uint32(1),
				"http2_error_code": http2.ErrCodeProtocol,
				"http2_cause":      "received from peer",
			},
		},
		{
			name: "goaway without debug data",
			err:  http2.GoAwayError{ErrCode: http2.ErrCodeNo, LastStreamID: 3},
			wantFields: logrus.Fields{
				"http2_error_code":     http2.ErrCodeNo,
				"http2_last_stream_id": uint32(3),
			},
			wantNoFields: []string{"http2_stream_id", "http2_debug"},
		},
		{
			name: "goaway with debug data",
			err:  http2.GoAwayError{ErrCode: http2.ErrCodeNo, LastStreamID: 3, DebugData: "server shutting down"},
			wantFields: logrus.Fields{
				"http2_error_code":     http2.ErrCodeNo,
				"http2_last_stream_id": uint32(3),
				"http2_debug":          "server shutting down",
			},
			wantNoFields: []string{"http2_stream_id"},
		},
		{
			name:         "generic error logs no http2 fields",
			err:          errors.New("connection refused"),
			wantNoFields: []string{"http2_stream_id", "http2_error_code", "http2_last_stream_id", "http2_cause", "http2_debug"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log := logrus.New()
			log.SetLevel(logrus.WarnLevel)
			hook := &captureHook{}
			log.AddHook(hook)

			logHTTP2RequestError(log, tc.err)

			require.Len(t, hook.entries, 1)
			entry := hook.entries[0]
			for k, v := range tc.wantFields {
				require.Equal(t, v, entry.Data[k], "field %q", k)
			}
			for _, k := range tc.wantNoFields {
				require.NotContains(t, entry.Data, k)
			}
		})
	}
}

type captureHook struct{ entries []*logrus.Entry }

func (h *captureHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *captureHook) Fire(e *logrus.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}
