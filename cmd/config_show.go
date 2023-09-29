package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
)

// configShowCmd represents the config show command
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current config",
	Long:  `Show the current config`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		fmt.Printf(`GRPC Endpoint: %s
FairyRing Node Endpoint: %s
Chain ID: %s
Chain Denom: %s
`, cfg.GetGRPCEndpoint(), cfg.GetFairyRingNodeURI(), cfg.FairyRingNode.ChainID, cfg.FairyRingNode.Denom)

		fmt.Printf("Share API Url: %s\n", cfg.ShareAPIUrl)
	},
}
