package cmd

import (
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fmt"
	"log"
	"time"

	"github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/spf13/cobra"
)

// delegateRemove represents the delegate add command
var delegateRemove = &cobra.Command{
	Use:   "remove [address]",
	Short: "Remove an authorized address for submitting key share",
	Long:  `Remove an authorized address for submitting key share`,
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

		msg := types.MsgDeleteAuthorizedAddress{
			Target:  args[0],
			Creator: eachClient.GetAddress(),
		}

		if err := msg.ValidateBasic(); err != nil {
			log.Fatalf("Invalid MsgDeleteAuthorizedAddress: %s", err.Error())
		}

		resp, err := eachClient.BroadcastTx(&msg, false)

		if err != nil {
			log.Fatalf("unable to broadcast delete authorized address message, ERROR: %s\n", err.Error())
		}

		txResp, err := eachClient.WaitForTx(resp.TxHash, time.Second)
		if err != nil {
			log.Fatalf("error on deleting authorized address: %s\n", err.Error())
		}

		if txResp.TxResponse.Code != 0 {
			log.Fatalf("Tx Failed with code: %d | Error Message: %s\n", txResp.TxResponse.Code, txResp.TxResponse.RawLog)
		}

		fmt.Printf("Successfully Deleted Authorized Address: %s submitting keyshare for address: %s | TXID: %s\n", args[0], eachClient.GetAddress(), txResp.TxResponse.TxHash)
	},
}
