package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"strconv"

	"github.com/KimMachineGun/automemlimit/memlimit"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/cmd"
	"castai-agent/internal/config"
)

const envGoMemLimitOverride = "GOMEMLIMIT_RATIO"

func init() {
	memlimitRatio := 0.8
	if ratioOverride := os.Getenv(envGoMemLimitOverride); ratioOverride != "" {
		val, err := strconv.ParseFloat(ratioOverride, 64)
		if err == nil {
			memlimitRatio = val
		} else {
			fmt.Fprintf(os.Stderr, "warning: failed to parse env var %q value %q as float: %v", envGoMemLimitOverride, ratioOverride, err)
		}
	}

	_, _ = memlimit.SetGoMemLimitWithOpts(
		memlimit.WithRatio(memlimitRatio),
		memlimit.WithProvider(memlimit.ApplyFallback(
			memlimit.FromCgroup,
			memlimit.FromSystem,
		)),
	)
}

// These should be set via `go build` during a release
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

func main() {
	ctx := signals.SetupSignalHandler()
	config.VersionInfo = &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}
	cmd.Execute(ctx)
}
