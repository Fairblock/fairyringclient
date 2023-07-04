package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// configDefaultCmd represents the config default command
var configDefaultCmd = &cobra.Command{
	Use:   "default",
	Short: "*Use with caution* Update config to default value",
	Long: `Update config to default value, private keys will be copied to new config.
However, backup is still highly recommended before using this command`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		defaultCfg := config.DefaultConfig(false)
		defaultCfg.PrivateKeys = cfg.PrivateKeys
		defaultCfg.MasterPrivateKey = cfg.MasterPrivateKey

		if err = defaultCfg.SaveConfig(); err != nil {
			fmt.Printf("Error saving config to the system: %s\n", err.Error())
			return
		}

		fmt.Println("Config successfully updated to default value!")
	},
}
