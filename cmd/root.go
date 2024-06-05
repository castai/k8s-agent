package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"castai-agent/cmd/agent"
	"castai-agent/cmd/dump"
	"castai-agent/cmd/monitor"
)

var rootCmd = &cobra.Command{
	Use:               "castai-agent",
	PersistentPreRunE: preRun,
}

var cfgFile string

func preRun(_ *cobra.Command, _ []string) error {
	if cfgFile == "" {
		if e := os.Getenv("CONFIG_PATH"); e != "" {
			cfgFile = e
		}
	}

	if cfgFile != "" {
		fmt.Println("Using config from a file", cfgFile)
		viper.SetConfigType("yaml")
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			return err
		}
	}

	return nil
}

func Execute(ctx context.Context) {
	// For backwards compatibility: if no command is provided try to get the "mode" from env vars
	cmd, _, err := rootCmd.Find(os.Args[1:])
	// default cmd if no cmd is given
	if err == nil && cmd.Use == rootCmd.Use && !errors.Is(cmd.Flags().Parse(os.Args[1:]), pflag.ErrHelp) {
		args := os.Args[1:]
		if mode := os.Getenv("MODE"); strings.ToLower(mode) == "monitor" {
			args = append([]string{monitor.Use}, os.Args[1:]...)
		} else {
			args = append([]string{agent.Use}, os.Args[1:]...)
		}
		rootCmd.SetArgs(args)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fatal(err)
	}
}

func init() {
	rootCmd.PersistentFlags().Int("log-level", 4, "Log level (0-5)")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig file")
	viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))

	rootCmd.PersistentFlags().String("provider", "", "Provider name")
	viper.BindPFlag("provider", rootCmd.PersistentFlags().Lookup("provider"))

	rootCmd.PersistentFlags().String("clusterId", "", "Cluster ID")
	viper.BindPFlag("static.cluster_id", rootCmd.PersistentFlags().Lookup("clusterid"))

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to agent config file")

	rootCmd.AddCommand(agent.NewCmd())
	rootCmd.AddCommand(monitor.NewCmd())
	rootCmd.AddCommand(dump.NewCmd())
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
