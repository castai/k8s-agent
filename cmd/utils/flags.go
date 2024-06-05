package utils

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func WithAPIFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("api-key", "", "")
	viper.BindPFlag("api.key", cmd.PersistentFlags().Lookup("api-key"))

	cmd.PersistentFlags().String("api-url", "", "")
	viper.BindPFlag("api.url", cmd.PersistentFlags().Lookup("api-url"))
}
