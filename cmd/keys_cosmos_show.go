package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// keysCosmosList represents the keys cosmos list command
var keysCosmosShow = &cobra.Command{
	Use:   "show",
	Short: "Show cosmos private key in config file",
	Long:  `Show cosmos private key in config file`,
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		fmt.Printf("Private Key: %s\n", cfg.PrivateKey)
	},
}
