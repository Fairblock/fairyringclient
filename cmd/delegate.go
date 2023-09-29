package cmd

import (
	"github.com/spf13/cobra"
)

// delegateCmd represents the delegate command
var delegateCmd = &cobra.Command{
	Use:   "delegate",
	Short: "Add / Remove another address for submitting KeyShare",
	Long:  `Add / Remove another address for submitting KeyShare`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(delegateCmd)
	delegateCmd.AddCommand(delegateAdd)
	delegateCmd.AddCommand(delegateRemove)
}
