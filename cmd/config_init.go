package cmd

import (
	"fairyringclient/config"
	"github.com/spf13/cobra"
)

// configInitCmd represents the config init command
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file for FairyRing Client",
	Long:  `Create default config & keys folder in your Home directory`,
	Run: func(cmd *cobra.Command, args []string) {
		withCosmosKey, _ := cmd.Flags().GetBool("with-cosmos-key")

		cfg := config.DefaultConfig(withCosmosKey)
		cfg.ExportConfig()
	},
}

func init() {
	configInitCmd.Flags().Bool("with-cosmos-key", false, "Initialize with random cosmos private key")
}
