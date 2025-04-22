package main

import (
	_ "net/http/pprof"

	"github.com/KimMachineGun/automemlimit/memlimit"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/cmd"
	"castai-agent/internal/config"
)

func init() {
	_, _ = memlimit.SetGoMemLimitWithOpts(
		memlimit.WithRatio(0.8),
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
