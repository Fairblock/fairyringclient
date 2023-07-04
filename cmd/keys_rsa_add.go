package cmd

import (
	"crypto/x509"
	"encoding/pem"
	"fairyringclient/config"
	"fairyringclient/pkg/shareAPIClient"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"strconv"
)

// keysRsaAdd represents the keys rsa add command
var keysRsaAdd = &cobra.Command{
	Use:   "add [rsa-private-key-file-path]",
	Short: "Command for adding RSA Keys",
	Long:  `Command for adding RSA Keys`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		privateKeyFileName := args[0]

		if _, err := os.Stat(privateKeyFileName); err != nil {
			fmt.Printf("Error locating RSA private key: %s\n", err.Error())
			return
		}

		// Loading the Private Key to rsa.PrivateKey
		pKey, err := shareAPIClient.PemToPrivateKey(privateKeyFileName)
		if err != nil {
			fmt.Printf("Error loading RSA private key: %s\n", err.Error())
			return
		}

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

		var validEntries uint64 = 0

		// Validate the files in the keys directory
		for _, file := range entries {
			match := skRe.FindStringSubmatch(file.Name())
			if len(match) < 2 {
				continue
			}

			validEntries++
		}

		fileNum := strconv.FormatUint(validEntries+1, 10)

		// Creating RSA Private Key
		pemPrivateKeyFile, err := os.Create(keysPath + "/sk" + fileNum + ".pem")
		if err != nil {
			fmt.Printf("Error creating RSA private key: %s\n", err.Error())
			return
		}

		defer pemPrivateKeyFile.Close()

		pemPrivateKey := pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(pKey),
		}

		if err = pem.Encode(pemPrivateKeyFile, &pemPrivateKey); err != nil {
			fmt.Printf("Error writing RSA private key: %s\n", err.Error())
			return
		}

		// Creating RSA Public Key
		pemPublicKey := pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(&pKey.PublicKey),
		}

		pemPublicKeyFile, err := os.Create(keysPath + "/pk" + fileNum + ".pem")
		if err != nil {
			fmt.Printf("Error creating RSA public key: %s\n", err.Error())
			return
		}

		defer pemPublicKeyFile.Close()

		if err = pem.Encode(pemPublicKeyFile, &pemPublicKey); err != nil {
			fmt.Printf("Error writing RSA public key: %s\n", err.Error())
			return
		}
	},
}
