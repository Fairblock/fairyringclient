package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
	"strconv"
)

// keysCosmosRemove represents the keys cosmos remove command
var keysCosmosRemove = &cobra.Command{
	Use:   "remove [private_key_index]",
	Short: "Remove specific private key in config file",
	Long:  `Remove specific private key in config file, to get the private key index, you can use "list"" command`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		index, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			fmt.Printf("Error parsing private key index: %s\n", err.Error())
			return
		}

		if index >= uint64(len(cfg.PrivateKeys)) {
			fmt.Printf("Invalid index provided: %d, expected index smaller than total number of private keys: %d\n", index, len(cfg.PrivateKeys))
			return
		}

		cfg.PrivateKeys = append(cfg.PrivateKeys[:index], cfg.PrivateKeys[index+1:]...)

		if err = cfg.SaveConfig(); err != nil {
			fmt.Printf("Error saving updated config to system: %s\n", err.Error())
			return
		}

		fmt.Println("Successfully removed specified cosmos private key in config!")
	},
}
