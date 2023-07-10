package castai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimer_Duration(t *testing.T) {
	timer := NewTimer()
	time.Sleep(time.Second)
	timer.Stop()

	duration := timer.Duration()
	require.GreaterOrEqual(t, duration, time.Millisecond*900)
	require.LessOrEqual(t, duration, time.Millisecond*1100)
}
