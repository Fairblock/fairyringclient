package cmd

import (
	"github.com/spf13/cobra"
)

// keysCmd represents the keys command
var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage RSA Keys & Cosmos private key",
	Long:  `Manage RSA Keys & Cosmos private key`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
		}
	},
}

// keysRsaCmd represents the keys rsa command
var keysRsaCmd = &cobra.Command{
	Use:   "rsa",
	Short: "Manage RSA Keys",
	Long:  `Manage RSA Keys`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
		}
	},
}

// keysCosmosCmd represents the keys cosmos command
var keysCosmosCmd = &cobra.Command{
	Use:   "cosmos",
	Short: "Manage Cosmos Private Keys",
	Long:  `Manage Cosmos Private Keys`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(keysCmd)

	keysCmd.AddCommand(keysRsaCmd)
	keysRsaCmd.AddCommand(keysRsaAdd)
	keysRsaCmd.AddCommand(keysRsaList)

	keysCmd.AddCommand(keysCosmosCmd)
	keysCosmosCmd.AddCommand(keysCosmosAdd)
	keysCosmosCmd.AddCommand(keysCosmosList)
	keysCosmosCmd.AddCommand(keysCosmosRemove)
}
