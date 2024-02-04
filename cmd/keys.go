package cmd

import (
	"github.com/spf13/cobra"
)

// keysCmd represents the keys command
var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage Cosmos private key",
	Long:  `Manage Cosmos private key`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(keysCmd)

	keysCmd.AddCommand(keysCosmosAdd)
	keysCmd.AddCommand(keysCosmosList)
	keysCmd.AddCommand(keysCosmosRemove)
}
