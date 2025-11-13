package health

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func HealthCheckHandler(checks map[string]healthz.Checker, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for name, checker := range checks {
			if err := checker(r); err != nil {
				log.WithField("check", name).Warnf("health check failed: %v", err)
				http.Error(w, fmt.Sprintf("%s check failed: %v", name, err), http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}
