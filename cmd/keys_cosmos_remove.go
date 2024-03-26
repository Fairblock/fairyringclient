package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// keysCosmosRemove represents the keys cosmos remove command
var keysCosmosRemove = &cobra.Command{
	Use:   "remove",
	Short: "Remove private key in config file",
	Long:  `Remove private key in config file`,
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		cfg.PrivateKey = ""

		if err = cfg.SaveConfig(); err != nil {
			fmt.Printf("Error saving updated config to system: %s\n", err.Error())
			return
		}

		fmt.Println("Successfully removed cosmos private key in config!")
	},
}
