package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

const ClientVersion = "v0.5.3"

// configCmd represents the config command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version of FairyRingClient",
	Long:  `Show version of FairyRingClient`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("FairyRingClient version: %s\n", ClientVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
