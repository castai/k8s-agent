package agent

import (
	"github.com/spf13/cobra"

	"castai-agent/cmd/utils"
)

const Use = "agent"

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: Use,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}

	utils.WithAPIFlags(cmd)

	return cmd
}
