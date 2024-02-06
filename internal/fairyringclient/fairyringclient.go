package fairyringclient

import (
	"context"
	"encoding/hex"
	"fairyring/x/keyshare/types"
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fairyringclient/pkg/shareAPIClient"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"strings"

	"log"
	"os"
	"strconv"
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

var (
	validatorCosmosClients []ValidatorClients
	pks                    []string
)

var (
	invalidShareSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fairyringclient_invalid_share_submitted",
		Help: "The total number of invalid key share submitted",
	})
	validShareSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fairyringclient_valid_share_submitted",
		Help: "The total number of valid key share submitted",
	})
	failedShareSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fairyringclient_failed_share_submitted",
		Help: "The total number of key share failed to submit",
	})
	currentShareExpiry = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fairyringclient_current_share_expiry",
		Help: "The expiry block of current key share",
	})
	latestProcessedHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fairyringclient_latest_processed_height",
		Help: "The latest height that submitted keyshare",
	})
)

func StartFairyRingClient(cfg config.Config) {

	Denom := cfg.FairyRingNode.Denom

	if len(Denom) == 0 {
		log.Fatal("Denom not found in config...")
	}

	gRPCEndpoint := cfg.GetGRPCEndpoint()

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

		bal, err := eachClient.GetBalance(Denom)
		if err != nil {
			log.Fatal("Error getting", eachClient.GetAddress(), "account balance: ", err)
		}
		log.Printf("Address: %s , Balance: %s %s\n", eachClient.GetAddress(), bal.String(), Denom)

		shareClient, err := shareAPIClient.NewShareAPIClient(
			cfg.ShareAPIUrl,
			eachPKey,
		)

		if err != nil {
			log.Fatal("Error creating share api client:", err)
		}

		validatorCosmosClients[index] = ValidatorClients{
			CosmosClient:   eachClient,
			ShareApiClient: shareClient,
		}

		allAccAddrs[index] = eachClient.GetAccAddress()

		privateKeyIndexNum++
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
		log.Printf("[%d] %s Registered as Validator", i, eachAddr)
	}

	for index, eachCosmosClient := range validatorCosmosClients {

		shareClient := eachCosmosClient.ShareApiClient
		eachClient := eachCosmosClient.CosmosClient

		pubKeys, err := eachClient.GetActivePubKey()
		if err != nil {
			if strings.Contains(err.Error(), "does not exists") {
				log.Println("Active pub key does not exists...")
				break
			}
			log.Fatal("Error getting active pub key on KeyShare module: ", err)
		}

		log.Printf("Active Pub Key: %s Expires at: %d | Queued: %s Expires at: %d\n",
			pubKeys.ActivePubKey.PublicKey,
			pubKeys.ActivePubKey.Expiry,
			pubKeys.QueuedPubKey.PublicKey,
			pubKeys.QueuedPubKey.Expiry,
		)

		share, shareIndex, err := shareClient.GetShare(getNowStr())

		if err != nil {
			if strings.Contains(err.Error(), "empty") {
				log.Printf("[%d] Couldn't find your share in API...", index)
				break
			}
			log.Fatal("Error getting share:", err)
		}
		log.Printf("Got share: %s | Index: %d", share, shareIndex)

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

				log.Printf("[%d] Updated pending share: %v, expires at block: %d", index, validatorCosmosClients[index].PendingShare, validatorCosmosClients[index].PendingShareExpiryBlock)
			}
		}

		commits, err := validatorCosmosClients[index].CosmosClient.GetCommitments()
		if err != nil {
			log.Fatal("Error getting commitments:", err)
		}

		validatorCosmosClients[index].SetCommitments(commits)

		log.Printf("[%d] Verifying Current Key Share...", index)

		valid, err := validatorCosmosClients[index].VerifyShare(commits.ActiveCommitments, false)
		if err != nil {
			log.Fatal("Error verifying active key share:", err)
		}
		if !valid {
			log.Printf("[%d] Active key share is invalid, Pausing the client...", index)
			validatorCosmosClients[index].Pause()
		} else {
			log.Printf("[%d] Current Key Share is valid !", index)
		}

		if validatorCosmosClients[index].PendingShare != nil && commits.QueuedCommitments != nil {
			log.Printf("[%d] Verifying Pending Key Share...", index)
			valid, err := validatorCosmosClients[index].VerifyShare(commits.QueuedCommitments, true)
			if err != nil {
				log.Fatal("Error verifying pending key share:", err)
			}
			if !valid {
				log.Printf("[%d] Pending key share is invalid...", index)
			} else {
				log.Printf("[%d] Pending Key Share is valid !", index)
			}
		}
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

	go handleTxEvents(txOut)

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Metrics is listening on port: %d\n", cfg.MetricsPort)
	go http.ListenAndServe(fmt.Sprintf(":%d", cfg.MetricsPort), nil)

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
					if nowEach.CurrentShare == nil {
						log.Printf("[%d] Current Share not found, client paused...", nowI)
						return
					}
					log.Printf("[%d] Current Share Expires at: %d, in %d blocks | %v", nowI, nowEach.CurrentShareExpiryBlock, nowEach.CurrentShareExpiryBlock-uint64(height), nowEach.CurrentShare.Share)
					if nowEach.PendingShare != nil {
						log.Printf("[%d] Pending Share expires at: %d, in %d blocks | %v", nowI, nowEach.PendingShareExpiryBlock, nowEach.PendingShareExpiryBlock-uint64(height), nowEach.PendingShare.Share)
					}
					// When it is time to switch key share
					if nowEach.CurrentShareExpiryBlock != 0 && nowEach.CurrentShareExpiryBlock <= processHeight {
						log.Printf("[%d] current share expired, trying to switch to the queued one...\n", nowI)
						validatorCosmosClients[nowI].RemoveCurrentShare()
						// But pending key share not found
						isVerified := false
						if nowEach.PendingShare == nil {
							log.Printf("[%d] Unable to switch to latest share, pending share not found, trying to get pending share...\n", nowI)
							valid, err := validatorCosmosClients[nowI].UpdateAndVerifyPendingShare(height)

							if err != nil {
								log.Printf("[%d] Error getting pending share from API.", nowI)
								return
							}

							if !valid {
								log.Printf("[%d] Got Invalid Share from API.", nowI)
								return
							}

							log.Printf("[%d] Successfully Got Valid Pending Share from API.", nowI)
							isVerified = true
						}

						if !isVerified {
							// Pending Share found, Verify before switching
							pendingShareValid, err := nowEach.VerifyShare(nowEach.Commitments.QueuedCommitments, true)
							if err != nil {
								log.Printf("[%d] Error verifying pending share: %s", nowI, err.Error())
								return
							}

							if !pendingShareValid {
								log.Printf("[%d] The existing pending share is invalid, trying to get a new one from API...", nowI)

								valid, err := validatorCosmosClients[nowI].UpdateAndVerifyPendingShare(height)

								if err != nil {
									log.Printf("[%d] Error getting pending share from API.", nowI)
									return
								}

								if !valid {
									log.Printf("[%d] Got Invalid Share from API.", nowI)
									return
								}

								log.Printf("[%d] Successfully Got Valid Pending Share from API.", nowI)
							}
						}

						log.Printf("[%d] The Existing Pending Share is Valid !", nowI)

						validatorCosmosClients[nowI].ResetInvalidShareNum()

						if validatorCosmosClients[nowI].Paused {
							validatorCosmosClients[nowI].Unpause()
							log.Printf("[%d] Client Unpaused, current invalid share count: %d...\n", nowI, nowEach.InvalidShareInARow)
						}

						validatorCosmosClients[nowI].ActivatePendingShare()
						log.Printf("[%d] Active share updated...\n", nowI)
						log.Printf("[%d] New Share: %v\n", nowI, validatorCosmosClients[nowI].CurrentShare)
					}

					defer currentShareExpiry.Set(float64(nowEach.CurrentShareExpiryBlock))

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

							defer invalidShareSubmitted.Inc()

							if nowEach.InvalidShareInARow >= PauseThreshold {
								validatorCosmosClients[nowI].Pause()
								log.Printf("[%d] Client paused due to number of invalid share in a row '%d' reaches threshold '%d', Waiting until next round...\n", nowI, nowEach.InvalidShareInARow, PauseThreshold)
							}

							return
						}

						if txResp.TxResponse.Code != 0 {
							log.Printf("[%d] KeyShare for Height %s Failed: %s\n", nowI, processHeightStr, txResp.TxResponse.RawLog)
							defer failedShareSubmitted.Inc()
							return
						}
						log.Printf("[%d] Submit KeyShare for Height %s Confirmed\n", nowI, processHeightStr)
						defer validShareSubmitted.Inc()

					}()
				}()
			}

			latestProcessedHeight.Set(float64(processHeight))
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

func handleTxEvents(txOut <-chan coretypes.ResultEvent) {
	for {
		select {
		case result := <-txOut:
			for k := range result.Events {
				switch k {
				case "start-send-general-keyshare.start-send-general-keyshare-identity":
					handleStartSubmitGeneralKeyShareEvent(result.Events)
					break
				case "queued-pubkey-created.queued-pubkey-created-pubkey":
					handleNewPubKeyEvent(result.Events)
					break
				}
			}
		}
	}
}

func handleStartSubmitGeneralKeyShareEvent(data map[string][]string) {
	id, found := data["start-send-general-keyshare.start-send-general-keyshare-identity"]
	if !found {
		return
	}

	if len(id) < 1 {
		return
	}

	identity := id[0]

	log.Printf("Start Submitting General Key Share for identity: %s", identity)
	s := bls.NewBLS12381Suite()
	for i, eachClient := range validatorCosmosClients {
		nowI := i
		nowClient := eachClient

		currentShare := nowClient.CurrentShare

		extractedKey := distIBE.Extract(s, currentShare.Share.Value, uint32(currentShare.Index), []byte(identity))
		extractedKeyBinary, err := extractedKey.SK.MarshalBinary()
		if err != nil {
			log.Fatal(err)
		}
		extractedKeyHex := hex.EncodeToString(extractedKeyBinary)

		log.Printf("Derived General Key Share: %s\n", extractedKeyHex)

		resp, err := nowClient.CosmosClient.BroadcastTx(&types.MsgCreateGeneralKeyShare{
			Creator:       nowClient.CosmosClient.GetAddress(),
			KeyShare:      extractedKeyHex,
			KeyShareIndex: currentShare.Index,
			IdType:        "private-gov-identity",
			IdValue:       identity,
		}, true)
		if err != nil {
			log.Printf("[%d] Submit General KeyShare for Identity %s ERROR: %s\n", nowI, identity, err.Error())
		}
		txResp, err := nowClient.CosmosClient.WaitForTx(resp.TxHash, time.Second)
		if err != nil {
			log.Printf("[%d] General KeyShare for Identity %s Failed: %s\n", nowI, identity, err.Error())
			return
		}
		if txResp.TxResponse.Code != 0 {
			log.Printf("[%d] General KeyShare for Identity %s Failed: %s\n", nowI, identity, txResp.TxResponse.RawLog)
			return
		}
		log.Printf("[%d] Submit General KeyShare for Identity %s Confirmed\n", nowI, identity)
	}
}

func handleNewPubKeyEvent(data map[string][]string) {
	pubKey, found := data["queued-pubkey-created.queued-pubkey-created-pubkey"]
	if !found {
		return
	}

	expiryHeightStr, found := data["queued-pubkey-created.queued-pubkey-created-expiry-height"]
	if !found {
		return
	}

	expiryHeight, err := strconv.ParseUint(expiryHeightStr[0], 10, 64)
	if err != nil {
		log.Printf("Error parsing pubkey expiry height: %s\n", err.Error())
		return
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

		if nowClient.CurrentShare == nil && nowClient.CurrentShareExpiryBlock == 0 {
			validatorCosmosClients[nowI].SetCurrentShare(&KeyShare{
				Share: *newShare,
				Index: index,
			})
			validatorCosmosClients[nowI].SetCurrentShareExpiryBlock(expiryHeight)
		} else {
			validatorCosmosClients[nowI].SetPendingShare(&KeyShare{
				Share: *newShare,
				Index: index,
			})
			validatorCosmosClients[nowI].SetPendingShareExpiryBlock(expiryHeight)
		}

		log.Printf("Got [%d] Client's New Share: %v | Expires at: %d\n", nowI, newShare.Value, expiryHeight)

		commits, err := validatorCosmosClients[nowI].CosmosClient.GetCommitments()
		for err != nil {
			if strings.Contains(err.Error(), "does not exists") {
				time.Sleep(5)
				commits, err = validatorCosmosClients[nowI].CosmosClient.GetCommitments()
			} else {
				log.Fatalf("Error getting commitments: %s", err.Error())
			}
		}

		validatorCosmosClients[nowI].SetCommitments(commits)
		log.Printf("[%d] Updated Commitments...", nowI)
	}
}
