package cmd

import (
	"encoding/hex"
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// keysCosmosAdd represents the keys cosmos add command
var keysCosmosSet = &cobra.Command{
	Use:   "set [private-key-in-hex]",
	Short: "Setting cosmos private key",
	Long:  `Setting cosmos private key to the config file`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		if len(args[0]) != 64 {
			fmt.Printf("Got invalid cosmos private key length, expected 64, got: %d\n", len(args[0]))
			return
		}

		if _, err = hex.DecodeString(args[0]); err != nil {
			fmt.Printf("Got invalid cosmos private key: %s\n", err.Error())
			return
		}

		cfg.PrivateKey = args[0]

		if err = cfg.SaveConfig(); err != nil {
			fmt.Printf("Error saving updated config to system: %s\n", err.Error())
			return
		}

		fmt.Println("Successfully added cosmos private key to config!")
	},
}
