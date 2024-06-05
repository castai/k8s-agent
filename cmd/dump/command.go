package dump

import "github.com/spf13/cobra"

var out string

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "dump-snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
		Long: `The dump-snapshot command provides a comprehensive snapshot of the current state of a Kubernetes cluster, encapsulating all relevant configurations and current states of resources. 
To successfully execute, the command requires read access to the entire cluster. 
This includes permissions to access all namespaces, pods, services, and other Kubernetes resources to ensure a complete and accurate snapshot. 
Users should ensure they have the necessary permissions before attempting to use this command.`,
	}

	cmd.PersistentFlags().StringVar(&out, "out", "",
		"Specifies the file path where the cluster snapshot will be saved. If this flag is not provided, the snapshot output will be directed to the standard output (stdout).")

	return cmd
}
