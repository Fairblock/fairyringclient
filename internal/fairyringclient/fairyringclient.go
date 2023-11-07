package fairyringclient

import (
	"context"
	"encoding/hex"
	"fairyring/x/keyshare/types"
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fairyringclient/pkg/shareAPIClient"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	distIBE "github.com/FairBlock/DistributedIBE"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	bls "github.com/drand/kyber-bls12381"

	tmclient "github.com/cometbft/cometbft/rpc/client/http"
	tmtypes "github.com/cometbft/cometbft/types"

	abciTypes "github.com/cometbft/cometbft/abci/types"
)

var (
	done      chan interface{}
	interrupt chan os.Signal
)

const PrivateKeyFileNameFormat = ".pem"

var (
	validatorCosmosClients []ValidatorClients
	pks                    []string
)

func StartFairyRingClient(cfg config.Config, keysDir string) {

	Denom := cfg.FairyRingNode.Denom

	if len(Denom) == 0 {
		log.Fatal("Denom not found in config...")
	}

	gRPCEndpoint := cfg.GetGRPCEndpoint()

	allPrivateKeys := cfg.PrivateKeys
	if len(allPrivateKeys) == 0 {
		log.Fatal("Private Keys Array is empty in config file, please add a valid cosmos account private key before starting")
	}

	validatorCosmosClients = make([]ValidatorClients, len(allPrivateKeys))

	log.Println("Loading total:", len(allPrivateKeys), "private key(s)")

	allAccAddrs := make([]cosmostypes.AccAddress, len(allPrivateKeys))

	privateKeyIndexNum := 1

	for index, eachPKey := range allPrivateKeys {
		eachClient, err := cosmosClient.NewCosmosClient(
			gRPCEndpoint,
			eachPKey,
			cfg.FairyRingNode.ChainID,
		)

		if err != nil {
			log.Fatal("Error creating custom cosmos client, make sure provided account is activated: ", err)
		}

		addr := eachClient.GetAddress()
		log.Printf("Validator Cosmos Client Loaded Address: %s\n", addr)

		shareClient, err := shareAPIClient.NewShareAPIClient(
			cfg.ShareAPIUrl,
			fmt.Sprintf(
				"%s/sk%d%s",
				keysDir,
				privateKeyIndexNum,
				PrivateKeyFileNameFormat,
			),
		)

		if err != nil {
			log.Fatal("Error creating share api client:", err)
		}

		privateKeyIndexNum++

		share, shareIndex, err := shareClient.GetShare(getNowStr())
		if err != nil {
			log.Fatal("Error getting share:", err)
		}
		log.Printf("Got share: %s | Index: %d", share, shareIndex)

		bal, err := eachClient.GetBalance(Denom)
		if err != nil {
			log.Fatal("Error getting", eachClient.GetAddress(), "account balance: ", err)
		}
		log.Printf("Address: %s , Balance: %s %s\n", eachClient.GetAddress(), bal.String(), Denom)

		validatorCosmosClients[index] = ValidatorClients{
			CosmosClient:   eachClient,
			ShareApiClient: shareClient,
			CurrentShare: &KeyShare{
				Share: *share,
				Index: shareIndex,
			},
		}

		allAccAddrs[index] = eachClient.GetAccAddress()

		pubKeys, err := eachClient.GetActivePubKey()
		if err != nil {
			log.Fatal("Error getting active pub key on pep module: ", err)
		}

		log.Printf("Active Pub Key: %s Expires at: %d | Queued: %s Expires at: %d\n",
			pubKeys.ActivePubKey.PublicKey,
			pubKeys.ActivePubKey.Expiry,
			pubKeys.QueuedPubKey.PublicKey,
			pubKeys.QueuedPubKey.Expiry,
		)

		validatorCosmosClients[index].SetCurrentShareExpiryBlock(pubKeys.ActivePubKey.Expiry)
		log.Println("Current Share Expiry Block set to: ", validatorCosmosClients[index].CurrentShareExpiryBlock)
		// Queued Pub key exists on pep module
		if len(pubKeys.QueuedPubKey.PublicKey) > 1 && pubKeys.QueuedPubKey.Expiry > 0 {
			previousShare, previousShareIndex, err := shareClient.GetLastShare(getNowStr())
			if err != nil {
				log.Fatal("Error getting previous share:", err)
			}
			log.Printf("[%d] Got previous share: %s | Index: %d", index, previousShare, previousShareIndex)

			if previousShare != nil {
				validatorCosmosClients[index].SetCurrentShare(&KeyShare{
					Share: *previousShare,
					Index: previousShareIndex,
				})

				log.Printf("[%d] Updated current share: %v", index, validatorCosmosClients[index].CurrentShare)

				validatorCosmosClients[index].SetPendingShare(&KeyShare{
					Share: *share,
					Index: shareIndex,
				})

				validatorCosmosClients[index].SetPendingShareExpiryBlock(pubKeys.QueuedPubKey.Expiry)
			}
		}

		commits, err := validatorCosmosClients[index].CosmosClient.GetCommitments()
		if err != nil {
			log.Fatal("Error getting commitments:", err)
		}

		log.Printf("[%d] Verifying Current Key Share...", index)

		valid, err := validatorCosmosClients[index].VerifyShare(commits, false)
		if err != nil {
			log.Fatal("Error verifying active key share:", err)
		}
		if !valid {
			log.Println("Active key share is invalid, Pausing the client...")
			validatorCosmosClients[index].Pause()
		}

		log.Printf("[%d] Current Key Share is valid !", index)

		if validatorCosmosClients[index].PendingShare != nil && commits.QueuedCommitments != nil {
			log.Printf("[%d] Verifying Pending Key Share...", index)
			valid, err := validatorCosmosClients[index].VerifyShare(commits, true)
			if err != nil {
				log.Fatal("Error verifying queued key share:", err)
			}
			if !valid {
				log.Println("Queued key share is invalid, error")
			}
			log.Printf("[%d] Pending Key Share is valid !", index)
		}

	}

	PauseThreshold := cfg.InvalidSharePauseThreshold

	client, err := tmclient.New(
		fmt.Sprintf(
			"%s://%s:%d",
			cfg.FairyRingNode.Protocol,
			cfg.FairyRingNode.IP,
			cfg.FairyRingNode.Port,
		),
		"/websocket",
	)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	for i, eachClient := range validatorCosmosClients {
		eachAddr := eachClient.CosmosClient.GetAddress()
		_, err = eachClient.CosmosClient.BroadcastTx(&types.MsgRegisterValidator{
			Creator: eachAddr,
		}, true)
		if err != nil {
			if !strings.Contains(err.Error(), "validator already registered") {
				log.Fatal(err)
			}
		}
		log.Printf("%d. %s Registered as Validator", i, eachAddr)
	}

	out, err := client.Subscribe(context.Background(), "", "tm.event = 'NewBlockHeader'")
	if err != nil {
		log.Fatal(err)
	}

	txOut, err := client.Subscribe(context.Background(), "", "tm.event = 'Tx'")
	if err != nil {
		log.Fatal(err)
	}

	defer client.Stop()

	s := bls.NewBLS12381Suite()

	go listenForNewPubKey(txOut)

	for {
		select {
		case result := <-out:
			newBlockHeader := result.Data.(tmtypes.EventDataNewBlockHeader)

			height := newBlockHeader.Header.Height
			fmt.Println("")

			processHeight := uint64(height + 1)
			processHeightStr := strconv.FormatUint(processHeight, 10)

			log.Printf("Latest Block Height: %d | Deriving Share for Height: %s\n", height, processHeightStr)

			for i, each := range validatorCosmosClients {
				nowI := i
				nowEach := each
				go func() {
					log.Printf("Current Share Expires at: %d | %v", nowEach.CurrentShareExpiryBlock, nowEach.CurrentShare.Share)
					if nowEach.CurrentShareExpiryBlock != 0 && nowEach.CurrentShareExpiryBlock <= processHeight {
						log.Printf("[%d] current share expired, trying to switch to the queued one...\n", nowI)
						if nowEach.PendingShare == nil {
							log.Printf("[%d] Unable to switch to latest share, pending share not found...\n", nowI)
							return
						}

						validatorCosmosClients[nowI].ActivatePendingShare()
						log.Printf("[%d] Active share updated...\n", nowI)
						log.Printf("[%d] New Share: %v\n", nowI, validatorCosmosClients[nowI].CurrentShare)

						commits, err := validatorCosmosClients[nowI].CosmosClient.GetCommitments()
						if err != nil {
							log.Fatal("Error getting commitments in switching key share:", err)
						}

						valid, err := validatorCosmosClients[nowI].VerifyShare(commits, false)
						if err != nil {
							log.Fatal("Error verifying active key share:", err)
						}
						if !valid {
							log.Printf("Active key share is invalid after switching key share, Pausing the client...\n")
							validatorCosmosClients[nowI].Pause()
						} else {
							validatorCosmosClients[nowI].Unpause()
							validatorCosmosClients[nowI].ResetInvalidShareNum()
							log.Printf("[%d] Client Unpaused, current invalid share count: %d...\n", nowI, nowEach.InvalidShareInARow)
						}
					}

					if validatorCosmosClients[nowI].Paused {
						log.Printf("[%d] Client paused, skip submitting keyshare for height %s, Waiting until next round...\n", nowI, processHeightStr)
						return
					}

					currentShare := validatorCosmosClients[nowI].CurrentShare

					extractedKey := distIBE.Extract(s, currentShare.Share.Value, uint32(currentShare.Index), []byte(processHeightStr))
					extractedKeyBinary, err := extractedKey.SK.MarshalBinary()
					if err != nil {
						log.Fatal(err)
					}
					extractedKeyHex := hex.EncodeToString(extractedKeyBinary)

					go func() {
						resp, err := nowEach.CosmosClient.BroadcastTx(&types.MsgSendKeyshare{
							Creator:       nowEach.CosmosClient.GetAddress(),
							Message:       extractedKeyHex,
							KeyShareIndex: currentShare.Index,
							BlockHeight:   processHeight,
						}, true)

						if err != nil {
							log.Printf("[%d] Submit KeyShare for Height %s ERROR: %s\n", nowI, processHeightStr, err.Error())
						}
						txResp, err := nowEach.CosmosClient.WaitForTx(resp.TxHash, time.Second)
						if err != nil {
							log.Printf("[%d] KeyShare for Height %s Failed: %s\n", nowI, processHeightStr, err.Error())
							return
						}

						if hasCoinSpentEvent(txResp.TxResponse.Events) {
							validatorCosmosClients[nowI].IncreaseInvalidShareNum()
							log.Printf("[%d] KeyShare for Height %s is INVALID, Got Slashed, Current number invalid share in a row: %d\n", nowI, processHeightStr, nowEach.InvalidShareInARow)

							if nowEach.InvalidShareInARow >= PauseThreshold {
								validatorCosmosClients[nowI].Pause()
								log.Printf("[%d] Client paused due to number of invalid share in a row '%d' reaches threshold '%d', Waiting until next round...\n", nowI, nowEach.InvalidShareInARow, PauseThreshold)
							}

							return
						}

						if txResp.TxResponse.Code != 0 {
							log.Printf("[%d] KeyShare for Height %s Failed: %s\n", nowI, processHeightStr, txResp.TxResponse.RawLog)
							return
						}
						log.Printf("[%d] Submit KeyShare for Height %s Confirmed\n", nowI, processHeightStr)

					}()
				}()
			}
		}
	}
}

func hasCoinSpentEvent(e []abciTypes.Event) bool {
	for _, eachEvent := range e {
		if eachEvent.Type == "coin_spent" {
			return true
		}
	}
	return false
}

func listenForNewPubKey(txOut <-chan coretypes.ResultEvent) {
	for {
		select {
		case result := <-txOut:
			pubKey, found := result.Events["queued-pubkey-created.queued-pubkey-created-pubkey"]
			if !found {
				continue
			}

			expiryHeightStr, found := result.Events["queued-pubkey-created.queued-pubkey-created-expiry-height"]
			if !found {
				continue
			}

			expiryHeight, err := strconv.ParseUint(expiryHeightStr[0], 10, 64)
			if err != nil {
				log.Printf("Error parsing pubkey expiry height: %s\n", err.Error())
				continue
			}

			log.Printf("New Pubkey found: %s | Expiry Height: %d\n", pubKey[0], expiryHeight)

			for i, eachClient := range validatorCosmosClients {
				nowI := i
				nowClient := eachClient
				newShare, index, err := nowClient.ShareApiClient.GetShare(getNowStr())
				if err != nil {
					log.Printf("[%d] Error getting the pending keyshare: %s", nowI, err.Error())
					return
				}
				validatorCosmosClients[nowI].SetPendingShare(&KeyShare{
					Share: *newShare,
					Index: index,
				})
				validatorCosmosClients[nowI].SetPendingShareExpiryBlock(expiryHeight)
				log.Printf("Got [%d] Client's New Share: %v | Expires at: %d\n", nowI, newShare.Value, expiryHeight)
			}
		}
	}
}
