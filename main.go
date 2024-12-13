package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/KimMachineGun/automemlimit/memlimit"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

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
	ctx := setupSignalHandler()
	config.VersionInfo = &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}
	cmd.Execute(ctx)
}

func setupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println(time.Now(), fmt.Sprintf("Gracefully shutting down..., got signal %s", <-sigChan))
		fmt.Println(time.Now(), "Cancelling the context")
		cancel()
	}()

	return ctx
}
