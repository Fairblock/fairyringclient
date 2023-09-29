package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// keysCosmosList represents the keys cosmos list command
var keysCosmosList = &cobra.Command{
	Use:   "list",
	Short: "List all cosmos private key in config file",
	Long:  `List all cosmos private key in config file`,
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		fmt.Printf("Found total %d private keys in config file\n", len(cfg.PrivateKeys))

		for i, pkey := range cfg.PrivateKeys {
			fmt.Printf("[%d] %s\n", i, pkey)
		}
	},
}
