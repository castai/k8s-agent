package health

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func TestHealthCheckHandler_VariousScenarios(t *testing.T) {
	cases := map[string]struct {
		checks      map[string]healthz.Checker
		wantStatus  int
		wantBody    string
		wantContain string
	}{
		"all checks pass": {
			checks: map[string]healthz.Checker{
				"check1": func(_ *http.Request) error { return nil },
				"check2": func(_ *http.Request) error { return nil },
			},
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		"one check fails": {
			checks: map[string]healthz.Checker{
				"check1": func(_ *http.Request) error { return nil },
				"check2": func(_ *http.Request) error { return fmt.Errorf("fail") },
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantContain: "check2 check failed: fail",
		},
		"all checks fail": {
			checks: map[string]healthz.Checker{
				"check1": func(_ *http.Request) error { return fmt.Errorf("fail1") },
				"check2": func(_ *http.Request) error { return fmt.Errorf("fail2") },
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantContain: "check failed: fail",
		},
		"empty checks map": {
			checks:     map[string]healthz.Checker{},
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		"nil checks map": {
			checks:     nil,
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)

			req, err := http.NewRequest(http.MethodGet, "/healthz", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			handler := HealthCheckHandler(tc.checks, log)
			handler.ServeHTTP(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantBody != "" {
				require.Equal(t, tc.wantBody, rr.Body.String())
			}
			if tc.wantContain != "" {
				require.Contains(t, rr.Body.String(), tc.wantContain)
			}
		})
	}
}
