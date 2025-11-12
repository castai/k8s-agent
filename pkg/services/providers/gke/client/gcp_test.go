package client

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegionFromZone(t *testing.T) {
	tests := []struct {
		zone   string
		region string
		err    error
	}{
		{
			zone:   "us-east1-a",
			region: "us-east1",
			err:    nil,
		},
		{
			zone:   "europe-west1-a",
			region: "europe-west1",
			err:    nil,
		},
		{
			zone:   "europe-west1",
			region: "",
			err:    fmt.Errorf("cannot parse provided zone %q to region", "europe-west1"),
		},
		{
			zone:   "",
			region: "",
			err:    errors.New("given zone input is empty"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.zone, func(t *testing.T) {
			r, err := regionFromZone(test.zone)
			require.Equal(t, test.err, err)
			require.Equal(t, test.region, r)
		})
	}
}
