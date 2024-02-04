package cmd

import (
	"fairyringclient/config"
	"fairyringclient/internal/fairyringclient"
	"fmt"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the client",
	Long:  `Start the client`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}
		fairyringclient.StartFairyRingClient(*cfg)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
