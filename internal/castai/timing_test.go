package castai

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTimer_Duration(t *testing.T) {
	timer := NewTimer()
	time.Sleep(time.Second)
	duration := timer.Duration()
	require.GreaterOrEqual(t, duration, time.Millisecond*900)
	require.LessOrEqual(t, duration, time.Millisecond*1100)
}
