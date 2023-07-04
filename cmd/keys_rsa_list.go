package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"regexp"
)

// keysRsaList represents the keys rsa list command
var keysRsaList = &cobra.Command{
	Use:   "list",
	Short: "List all RSA Keys found in keys directory",
	Long:  `List all RSA Keys found in keys directory`,
	Run: func(cmd *cobra.Command, args []string) {
		// Getting the $HOME path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting $HOME directory: %s\n", err.Error())
			return
		}

		keysPath := homeDir + "/" + config.DefaultKeyFolderName

		entries, err := os.ReadDir(keysPath)
		if err != nil {
			fmt.Printf("Error reading keys directory: %s\n", err.Error())
			return
		}

		skRe, err := regexp.Compile(`sk([0-9]+)\.pem`)
		if err != nil {
			fmt.Printf("Error compiling regular expression: %s\n", err.Error())
			return
		}

		var validEntries []os.DirEntry

		// Validate the files in the keys directory
		for _, file := range entries {
			match := skRe.FindStringSubmatch(file.Name())
			if len(match) < 2 {
				continue
			}

			validEntries = append(validEntries, file)
		}

		fmt.Printf("Found total %d RSA private keys in keys directory %s\n", len(validEntries), keysPath)

		for i, each := range validEntries {
			fmt.Printf("[%d] %s\n", i, each.Name())
		}

	},
}
