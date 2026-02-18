package fairyringclient

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fairyringclient/config"
	"fairyringclient/pkg/cosmosClient"
	"fmt"
	"net/http"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/pkg/errors"

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
	latestSubmitKeyshare = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fairyringclient_latest_submit_keyshare_height",
		Help: "Get latest submit keyshare block height",
	})
)

func StartFairyRingClient(cfg config.Config) {

	PauseThreshold := cfg.InvalidSharePauseThreshold

	vCosmosClient, client, err := InitializeValidatorClient(cfg)
	if err != nil {
		log.Fatal(err)
	}

	validatorCosmosClient = vCosmosClient

	if !validatorCosmosClient.IsAccountAuthorized() {
		validatorCosmosClient.RegisterValidatorSet()
	} else {
		log.Println("Account is Authorized, skip registering in keyshare module.")
	}

	_ = validatorCosmosClient.UpdateKeyShareFromChain(false)
	_ = validatorCosmosClient.UpdateKeyShareFromChain(true)

	out, err := client.Subscribe(context.Background(), "", "tm.event = 'NewBlock'")
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

	go func() {
		if err := validatorCosmosClient.CosmosClient.HandleTxQueue(); err != nil {
			log.Printf("Error in queued tx handler: %v", err)
		}
	}()

	for {
		select {
		case result := <-out:
			newBlock := result.Data.(tmtypes.EventDataNewBlock)

			height := newBlock.Block.Height
			fmt.Println("")

			totalEventList := newBlock.ResultFinalizeBlock.Events
			for _, txResult := range newBlock.ResultFinalizeBlock.TxResults {
				totalEventList = append(totalEventList, txResult.Events...)
			}

			go handleEndBlockEvents(totalEventList)

			processHeight := uint64(height + 1)
			processHeightStr := strconv.FormatUint(processHeight, 10)

			log.Printf("Latest Block Height: %d | Deriving Share for Height: %s\n", height, processHeightStr)

			if validatorCosmosClient.CurrentShare == nil {
				log.Println("Current Share not found, Getting Share from FairyRing")
				if err := validatorCosmosClient.UpdateKeyShareFromChain(false); err != nil {
					continue
				}
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
				log.Println("Current share expired, Switching to the queued one")
				validatorCosmosClient.RemoveCurrentShare()

				// But pending key share not found
				if validatorCosmosClient.PendingShare == nil {
					log.Println("Pending share not found, Getting share from FairyRing now")
					if err = validatorCosmosClient.UpdateKeyShareFromChain(true); err != nil {
						continue
					}
				}

				validatorCosmosClient.ResetInvalidShareNum()

				if validatorCosmosClient.Paused {
					validatorCosmosClient.Unpause()
					log.Printf("Client Unpaused, Current invalid share count: %d\n", validatorCosmosClient.InvalidShareInARow)
				}

				validatorCosmosClient.ActivatePendingShare()
				log.Printf("Activated pending key share, New Share: %v\n", validatorCosmosClient.CurrentShare.Share.Value.String())
			}

			go func() {
				defer currentShareExpiry.Set(float64(validatorCosmosClient.CurrentShareExpiryBlock))
			}()

			if validatorCosmosClient.Paused {
				log.Printf("Client paused, Skip submitting keyshare for height %s, Waiting until next round\n", processHeightStr)
				return
			}

			extractedKeyHex, keyShareIndex, err := validatorCosmosClient.DeriveKeyShare([]byte(processHeightStr))
			if err != nil {
				log.Fatal(err)
			}

			validatorCosmosClient.CosmosClient.AddTxToQueue(&types.MsgSendKeyshare{
				Creator:       validatorCosmosClient.CosmosClient.GetAddress(),
				Message:       extractedKeyHex,
				KeyshareIndex: keyShareIndex,
				BlockHeight:   processHeight,
			}, true,
				func(err error) {
					log.Printf("Submit KeyShare for Height %s ERROR: %s\n", processHeightStr, err.Error())
					if strings.Contains(err.Error(), "transaction indexing is disabled") {
						log.Fatal("Transaction indexing is disabled on the node, please enable it or use another node with tx indexing, exiting FairyRingClient")
					}
					if strings.Contains(err.Error(), "account sequence mismatch") {
						log.Println("Account sequence mismatch, when submitting keyshares")
					}
				},
				func(txResp *tx.GetTxResponse) {
					if hasCoinSpentEvent(txResp.TxResponse.Events) {
						validatorCosmosClient.IncreaseInvalidShareNum()
						log.Printf("KeyShare for Height %s is INVALID, Got Slashed, Current number invalid share in a row: %d\n", processHeightStr, validatorCosmosClient.InvalidShareInARow)

						defer invalidShareSubmitted.Inc()

						if validatorCosmosClient.InvalidShareInARow >= PauseThreshold {
							validatorCosmosClient.Pause()
							log.Printf("Client paused due to number of invalid share in a row '%d' reaches threshold '%d', Waiting until next round\n", validatorCosmosClient.InvalidShareInARow, PauseThreshold)
						}

						return
					}

					if txResp.TxResponse.Code != 0 {
						log.Printf("KeyShare for Height %s Failed: %s\n", processHeightStr, txResp.TxResponse.RawLog)
						defer failedShareSubmitted.Inc()
						return
					}
					log.Printf("Submit KeyShare for Height %s Confirmed\n", processHeightStr)
					latestSubmitKeyshare.Set(float64(processHeight))
					defer validShareSubmitted.Inc()
				})

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
				case "pubkey-overrode.pubkey":
					handlePubKeyOverrodeEvent(result.Events)
					break
				}
			}
		}
	}
}

func handleEndBlockEvents(events []abciTypes.Event) {
	for _, e := range events {
		if e.Type == "start-send-encrypted-keyshare" {
			var id, secpPubkey, requester string
			for _, a := range e.Attributes {
				if a.Key == "identity" {
					id = a.Value
				}
				if a.Key == "requester" {
					requester = a.Value
				}
				if a.Key == "secp256k1-pubkey" {
					secpPubkey = a.Value
				}
			}

			if len(id) < 1 {
				log.Printf("Empty Identity detected in start send private key share event")
				continue
			}

			handleStartSubmitEncryptedKeyShareEvent(id, secpPubkey, requester)
			continue
		}

		if e.Type != "start-send-general-keyshare" {
			continue
		}
		for _, a := range e.Attributes {

			if a.Key != "identity" {
				continue
			}

			identity := a.Value
			if len(identity) < 1 {
				log.Printf("Empty Identity detected in start send general key share event")
				return
			}

			handleStartSubmitGeneralKeyShareEvent(identity)
			return
		}
	}
}

func handleStartSubmitEncryptedKeyShareEvent(
	identity string,
	secpPubkey string,
	requester string,
) {
	log.Printf("Start Submitting Encrypted Key Share for identity: %s pubkey: %s requester: %s", identity, secpPubkey, requester)
	derivedShare, index, err := validatorCosmosClient.DeriveKeyShare([]byte(identity))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Derived Private Key Share: %s\n", derivedShare)

	// Encrypt the message
	encryptedMessage, err := encryptWithPublicKey(derivedShare, secpPubkey)
	if err != nil {
		fmt.Printf("Error encrypting message: %s\n", err)
		return
	}

	validatorCosmosClient.CosmosClient.AddTxToQueue(&types.MsgSubmitEncryptedKeyshare{
		Creator:           validatorCosmosClient.CosmosClient.GetAddress(),
		Identity:          identity,
		KeyshareIndex:     index,
		Requester:         requester,
		EncryptedKeyshare: encryptedMessage,
		SecpPubkey:        secpPubkey,
	}, true,
		func(err error) {
			log.Printf("Submit Private KeyShare for Identity %s Requester %s Failed: %s\n", identity, requester, err.Error())
		},
		func(txResp *tx.GetTxResponse) {
			if txResp.TxResponse.Code != 0 {
				log.Printf("Private KeyShare for Identity %s Requester %s Failed: %s\n", identity, requester, txResp.TxResponse.RawLog)
				return
			} else {
				log.Printf("Private KeyShare for Identity %s Requester %s Confirmed\n", identity, requester)
			}
		})
}

// This function encrypts data using an RSA public key.
func encryptWithPublicKey(data string, pubKeyBase64 string) (string, error) {
	// Decode the base64 public key
	pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return "", err
	}

	// Load the secp256k1 public key
	pubKey, err := btcec.ParsePubKey(pubKeyBytes, btcec.S256())
	if err != nil {
		return "", err
	}

	ciphertext, err := btcec.Encrypt(pubKey, []byte(data))
	if err != nil {
		return "", err
	}

	// Encode ciphertext as hex for easy handling
	return hex.EncodeToString(ciphertext), nil
}

func handleStartSubmitGeneralKeyShareEvent(identity string) {
	log.Printf("Start Submitting General Key Share for identity: %s", identity)
	derivedShare, index, err := validatorCosmosClient.DeriveKeyShare([]byte(identity))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Derived General Key Share: %s\n", derivedShare)

	validatorCosmosClient.CosmosClient.AddTxToQueue(&types.MsgSubmitGeneralKeyshare{
		Creator:       validatorCosmosClient.CosmosClient.GetAddress(),
		Keyshare:      derivedShare,
		KeyshareIndex: index,
		IdType:        "private-gov-identity",
		IdValue:       identity,
	}, true,
		func(err error) {
			log.Printf("Submit General KeyShare for Identity %s ERROR: %s\n", identity, err.Error())
			if strings.Contains(err.Error(), "account sequence") {
				go func(id string) {
					handleStartSubmitGeneralKeyShareEvent(id)
				}(identity)
			}
		},
		func(txResp *tx.GetTxResponse) {
			if txResp.TxResponse.Code != 0 {
				log.Printf("General KeyShare for Identity %s Failed: %s\n", identity, txResp.TxResponse.RawLog)
				return
			} else {
				log.Printf("Submit General KeyShare for Identity %s Confirmed\n", identity)
			}
		})
}

func handlePubKeyOverrodeEvent(data map[string][]string) {
	pubKey, found := data["pubkey-overrode.pubkey"]
	if !found {
		return
	}

	log.Printf("Old Pubkey Overrode, New Pubkey found: %s\n", pubKey[0])

	for {
		err := validatorCosmosClient.UpdateKeyShareFromChain(false)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		log.Printf(
			"Successfully Updated Shares for the current overrode round: %s | Index: %d",
			validatorCosmosClient.CurrentShare.Share.Value.String(),
			validatorCosmosClient.CurrentShare.Index,
		)
		validatorCosmosClient.RemovePendingShare()
		break
	}
}

func handleNewPubKeyEvent(data map[string][]string) {
	pubKey, found := data["queued-pubkey-created.pubkey"]
	if !found {
		return
	}

	log.Printf("New Pubkey found: %s\n", pubKey[0])

	// Get Share & Commits on chain few blocks later
	for {
		err := validatorCosmosClient.UpdateKeyShareFromChain(true)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		log.Printf(
			"Successfully Updated Shares for next round: %s | Index: %d",
			validatorCosmosClient.PendingShare.Share.Value.String(),
			validatorCosmosClient.PendingShare.Index,
		)
		break
	}
}
