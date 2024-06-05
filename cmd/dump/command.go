package dump

import "github.com/spf13/cobra"

var out string

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "dump-snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}

	cmd.PersistentFlags().StringVar(&out, "out", "", "output file path")

	return cmd
}
