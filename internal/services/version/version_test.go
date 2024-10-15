package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func Test(t *testing.T) {
	v := version.Info{
		Major:      "1",
		Minor:      "21+",
		GitVersion: "v1.21.0",
		GitCommit:  "2812f9fb0003709fc44fc34166701b377020f1c9",
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Errorf("unexpected encoding error: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(b)
		require.NoError(t, err)
	}))
	defer s.Close()
	client := kubernetes.NewForConfigOrDie(&rest.Config{Host: s.URL})

	got, err := Get(logrus.New(), client)
	if err != nil {
		return
	}

	require.NoError(t, err)
	require.Equal(t, "1.21.0", got.Full())
	require.Equal(t, 21, got.MinorInt())
}
