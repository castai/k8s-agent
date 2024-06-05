package main

import (
	"context"
	_ "net/http/pprof"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/cmd"
	"castai-agent/internal/config"
)

// These should be set via `go build` during a release
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

func main() {
	ctx := signals.SetupSignalHandler()
	ctx = context.WithValue(ctx, "agentVersion", &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	})
	cmd.Execute(ctx)
}
