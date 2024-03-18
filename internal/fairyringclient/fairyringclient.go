package fairyringclient

import (
	"context"
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"strings"

	"github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"log"
	"strconv"
	"time"

	tmclient "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"

	abciTypes "github.com/cometbft/cometbft/abci/types"
)

var (
	validatorCosmosClient *ValidatorClients
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

	PauseThreshold := cfg.InvalidSharePauseThreshold

	vCosmosClient, client, err := InitializeValidatorClient(cfg)
	if err != nil {
		log.Fatal(err)
	}

	validatorCosmosClient = vCosmosClient

	validatorCosmosClient.RegisterValidatorSet()

	pubKeys, err := validatorCosmosClient.CosmosClient.GetActivePubKey()
	if err != nil {
		if strings.Contains(err.Error(), "does not exists") {
			log.Println("Active pub key does not exists...")
		}
		log.Fatal("Error getting active pub key on KeyShare module: ", err)
	}

	log.Printf("Active Pub Key: %s Expires at: %d | Queued: %s Expires at: %d\n",
		pubKeys.ActivePubKey.PublicKey,
		pubKeys.ActivePubKey.Expiry,
		pubKeys.QueuedPubKey.PublicKey,
		pubKeys.QueuedPubKey.Expiry,
	)

	err = validatorCosmosClient.UpdateKeyShareFromChain(false)
	if err != nil {
		log.Fatalf("Error getting current key share from the chain when starting the client: %s", err.Error())
	}
	log.Println("Successfully Updated Key Share for current round")

	// Queued Pub key exists on pep module
	if len(pubKeys.QueuedPubKey.PublicKey) > 1 && pubKeys.QueuedPubKey.Expiry > 0 {
		err = validatorCosmosClient.UpdateKeyShareFromChain(true)
		if err != nil {
			log.Printf("Error getting pending key share from the chain when starting the client: %s\n", err.Error())
		} else {
			log.Println("Successfully Updated Key Share for the next round")
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

			go handleEndBlockEvents(newBlockHeader.ResultEndBlock.GetEvents())

			processHeight := uint64(height + 1)
			processHeightStr := strconv.FormatUint(processHeight, 10)

			log.Printf("Latest Block Height: %d | Deriving Share for Height: %s\n", height, processHeightStr)

			if validatorCosmosClient.CurrentShare == nil {
				log.Println("Current Share not found, client paused...")
				return
			}
			log.Printf("Current Share Expires at: %d, in %d blocks | %v",
				validatorCosmosClient.CurrentShareExpiryBlock,
				validatorCosmosClient.CurrentShareExpiryBlock-uint64(height),
				validatorCosmosClient.CurrentShare.Share,
			)
			if validatorCosmosClient.PendingShare != nil {
				log.Printf("Pending Share expires at: %d, in %d blocks | %v",
					validatorCosmosClient.PendingShareExpiryBlock,
					validatorCosmosClient.PendingShareExpiryBlock-uint64(height),
					validatorCosmosClient.PendingShare.Share,
				)
			}
			// When it is time to switch key share
			if validatorCosmosClient.CurrentShareExpiryBlock != 0 && validatorCosmosClient.CurrentShareExpiryBlock <= processHeight {
				log.Println("current share expired, trying to switch to the queued one...")
				validatorCosmosClient.RemoveCurrentShare()

				// But pending key share not found
				if validatorCosmosClient.PendingShare == nil {
					log.Println("Unable to switch to latest share, pending share not found, trying to get pending share...")
					err := validatorCosmosClient.UpdateKeyShareFromChain(true)
					if err != nil {
						log.Printf("Error getting pending share from API: %s", err.Error())
						continue
					}

					log.Println("Successfully Got Valid Pending Share from the chain.")
					log.Println(validatorCosmosClient.PendingShare)
				}

				validatorCosmosClient.ResetInvalidShareNum()

				if validatorCosmosClient.Paused {
					validatorCosmosClient.Unpause()
					log.Printf("Client Unpaused, current invalid share count: %d...\n", validatorCosmosClient.InvalidShareInARow)
				}

				validatorCosmosClient.ActivatePendingShare()
				log.Println("Activated pending key share, active share updated...")
				log.Printf("New Share: %v\n", validatorCosmosClient.CurrentShare)
			}

			go currentShareExpiry.Set(float64(validatorCosmosClient.CurrentShareExpiryBlock))

			if validatorCosmosClient.Paused {
				log.Printf("Client paused, skip submitting keyshare for height %s, Waiting until next round...\n", processHeightStr)
				return
			}

			extractedKeyHex, keyShareIndex, err := validatorCosmosClient.DeriveKeyShare([]byte(processHeightStr))
			if err != nil {
				log.Fatal(err)
			}

			go func() {
				resp, err := validatorCosmosClient.CosmosClient.BroadcastTx(&types.MsgSendKeyshare{
					Creator:       validatorCosmosClient.CosmosClient.GetAddress(),
					Message:       extractedKeyHex,
					KeyShareIndex: keyShareIndex,
					BlockHeight:   processHeight,
				}, true)

				if err != nil {
					log.Printf("Submit KeyShare for Height %s ERROR: %s\n", processHeightStr, err.Error())
					if strings.Contains(err.Error(), "transaction indexing is disabled") {
						log.Fatal("Transaction indexing is disabled on the node, please enable it or use another node with tx indexing, exiting fairyringclient...")
					}
					if strings.Contains(err.Error(), "account sequence mismatch") {
						log.Fatal("Account sequence mismatch, exiting fairyringclient...")
					}
					return
				}
				txResp, err := validatorCosmosClient.CosmosClient.WaitForTx(resp.TxHash, time.Second)
				if err != nil {
					log.Printf("KeyShare for Height %s Failed: %s\n", processHeightStr, err.Error())
					return
				}

				if hasCoinSpentEvent(txResp.TxResponse.Events) {
					validatorCosmosClient.IncreaseInvalidShareNum()
					log.Printf("KeyShare for Height %s is INVALID, Got Slashed, Current number invalid share in a row: %d\n", processHeightStr, validatorCosmosClient.InvalidShareInARow)

					defer invalidShareSubmitted.Inc()

					if validatorCosmosClient.InvalidShareInARow >= PauseThreshold {
						validatorCosmosClient.Pause()
						log.Printf("Client paused due to number of invalid share in a row '%d' reaches threshold '%d', Waiting until next round...\n", validatorCosmosClient.InvalidShareInARow, PauseThreshold)
					}

					return
				}

				if txResp.TxResponse.Code != 0 {
					log.Printf("KeyShare for Height %s Failed: %s\n", processHeightStr, txResp.TxResponse.RawLog)
					defer failedShareSubmitted.Inc()
					return
				}
				log.Printf("Submit KeyShare for Height %s Confirmed\n", processHeightStr)
				defer validShareSubmitted.Inc()

			}()

			latestProcessedHeight.Set(float64(processHeight))
		}
	}
}

func InitializeValidatorClient(cfg config.Config) (*ValidatorClients, *tmclient.HTTP, error) {
	denom := cfg.FairyRingNode.Denom

	if len(denom) == 0 {
		return nil, nil, errors.New("denom not found in config")
	}

	gRPCEndpoint := cfg.GetGRPCEndpoint()

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
		return nil, nil, err
	}

	if err = client.Start(); err != nil {
		return nil, nil, err
	}

	if len(cfg.PrivateKey) == 0 {
		log.Fatal("Private Key is empty in config file, please add a valid cosmos account private key before starting")
	}

	vCosmosClient, err := cosmosClient.NewCosmosClient(
		gRPCEndpoint,
		cfg.PrivateKey,
		cfg.FairyRingNode.ChainID,
	)

	if err != nil {
		return nil, nil, errors.Wrap(err, "error creating custom cosmos client, make sure provided account is activated")
	}

	addr := vCosmosClient.GetAddress()
	log.Printf("Validator Cosmos Client Loaded Address: %s\n", addr)

	bal, err := vCosmosClient.GetBalance(denom)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting account balance")
	}
	log.Printf("Address: %s , Balance: %s %s\n", vCosmosClient.GetAddress(), bal.String(), denom)

	return &ValidatorClients{CosmosClient: vCosmosClient}, client, nil
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
				case "queued-pubkey-created.pubkey":
					handleNewPubKeyEvent(result.Events)
					break
				}
			}
		}
	}
}

func handleEndBlockEvents(events []abciTypes.Event) {
	for _, e := range events {
		if e.Type != "start-send-general-keyshare" {
			continue
		}
		for _, a := range e.Attributes {

			if a.Key != "start-send-general-keyshare-identity" {
				continue
			}

			identity := a.Value
			if len(identity) < 1 {
				log.Printf("Empty Identity detected in start send general key share event...")
				return
			}

			handleStartSubmitGeneralKeyShareEvent(identity)
			return
		}
	}
}

func handleStartSubmitGeneralKeyShareEvent(identity string) {
	log.Printf("Start Submitting General Key Share for identity: %s", identity)
	derivedShare, index, err := validatorCosmosClient.DeriveKeyShare([]byte(identity))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Derived General Key Share: %s\n", derivedShare)

	resp, err := validatorCosmosClient.CosmosClient.BroadcastTx(&types.MsgCreateGeneralKeyShare{
		Creator:       validatorCosmosClient.CosmosClient.GetAddress(),
		KeyShare:      derivedShare,
		KeyShareIndex: index,
		IdType:        "private-gov-identity",
		IdValue:       identity,
	}, true)
	if err != nil {
		log.Printf("Submit General KeyShare for Identity %s ERROR: %s\n", identity, err.Error())
	}
	txResp, err := validatorCosmosClient.CosmosClient.WaitForTx(resp.TxHash, time.Second)
	if err != nil {
		log.Printf("General KeyShare for Identity %s Failed: %s\n", identity, err.Error())
		return
	}
	if txResp.TxResponse.Code != 0 {
		log.Printf("General KeyShare for Identity %s Failed: %s\n", identity, txResp.TxResponse.RawLog)
		return
	}
	log.Printf("Submit General KeyShare for Identity %s Confirmed\n", identity)
}

func handleNewPubKeyEvent(data map[string][]string) {
	pubKey, found := data["queued-pubkey-created.pubkey"]
	if !found {
		return
	}

	expiryHeightStr, found := data["queued-pubkey-created.active-pubkey-expiry-height"]
	if !found {
		return
	}

	expiryHeight, err := strconv.ParseUint(expiryHeightStr[0], 10, 64)
	if err != nil {
		log.Printf("Error parsing pubkey expiry height: %s\n", err.Error())
		return
	}

	log.Printf("New Pubkey found: %s | Expiry Height: %d\n", pubKey[0], expiryHeight)

	// Get Share & Commits on chain few blocks later
	go func() {
		log.Println("Getting Key Share from chain in 15 seconds...")
		time.Sleep(15 * time.Second)
		err := validatorCosmosClient.UpdateKeyShareFromChain(true)
		if err != nil {
			log.Fatalf("Error when updating key shares on new pub key event: %s", err.Error())
		}

		log.Printf(
			"Successfully Updated Shares for next round: %s | Index: %d",
			validatorCosmosClient.PendingShare.Share.Value.String(),
			validatorCosmosClient.PendingShare.Index,
		)

		log.Printf("New Pending Commitments: %v", validatorCosmosClient.Commitments.QueuedCommitments.Commitments)
	}()
}
