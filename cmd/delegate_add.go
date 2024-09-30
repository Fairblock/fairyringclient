package cmd

import (
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fmt"
	"github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/spf13/cobra"
	"log"
)

// delegateAdd represents the delegate add command
var delegateAdd = &cobra.Command{
	Use:   "add [address]",
	Short: "Authorize an address for submitting key share",
	Long:  `Authorize an address for submitting key share`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		gRPCEndpoint := cfg.GetGRPCEndpoint()

		if len(cfg.PrivateKey) == 0 {
			log.Fatal("Private Key is empty in config file, please add a valid cosmos account private key before starting")
		}

		eachClient, err := cosmosClient.NewCosmosClient(
			gRPCEndpoint,
			cfg.PrivateKey,
			cfg.FairyRingNode.ChainID,
		)

		if err != nil {
			log.Fatalf("Error creating custom cosmos client, make sure provided account is activated: %v\n", err)
		}

		msg := types.MsgCreateAuthorizedAddress{
			Target:  args[0],
			Creator: eachClient.GetAddress(),
		}

		if err := msg.ValidateBasic(); err != nil {
			log.Fatalf("Invalid MsgCreateAuthorizedAddress: %s", err.Error())
		}

		txResp, err := eachClient.BroadcastTx(&msg, false)

		if err != nil {
			log.Fatalf("unable to broadcast create authorized address message, ERROR: %s\n", err.Error())
		}

		if txResp.TxResponse.Code != 0 {
			log.Fatalf("Tx Failed with code: %d | Error Message: %s\n", txResp.TxResponse.Code, txResp.TxResponse.RawLog)
		}

		fmt.Printf("Successfully Delegated Address: %s submitting keyshare for address: %s | TXID: %s\n", args[0], eachClient.GetAddress(), txResp.TxResponse.TxHash)
	},
}
