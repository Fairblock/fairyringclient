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

		// share, shareIndex, err := shareClient.GetShare(getNowStr())

		hexShare := "29c861be5016b20f5a4397795e3f086d818b11ad02e0dd8ee28e485988b6cb07"
		shareByte, _ := hex.DecodeString(hexShare)

		parsedShare := bls.NewKyberScalar()
		err = parsedShare.UnmarshalBinary(shareByte)

		var shareIndex uint64 = 1

		var share = &distIBE.Share{
			Index: bls.NewKyberScalar().SetInt64(int64(1)),
			Value: parsedShare,
		}

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
	}

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
		txResp, err := eachClient.CosmosClient.BroadcastTx(&types.MsgRegisterValidator{
			Creator: eachAddr,
		}, true)
		if err != nil {
			if !strings.Contains(err.Error(), "validator already registered") {
				log.Fatal(err)
			}
		}
		resp, err := eachClient.CosmosClient.WaitForTx(txResp.TxHash, time.Second)
		if err != nil {
			log.Fatalf("Error while waiting for the register validator tx to be confirmed: %s", err.Error())
		}
		if resp.TxResponse.Code == 0 {
			log.Printf("%d. %s Successfully Registered as Validator", i, eachAddr)
		} else {
			log.Fatalf("Register Validator tx failed with code: %d | Raw Log: %s", resp.TxResponse.Code, resp.TxResponse.RawLog)
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
	txOut2, err := client.Subscribe(context.Background(), "", "tm.event = 'Tx'")
	if err != nil {
		log.Fatal(err)
	}

	defer client.Stop()

	s := bls.NewBLS12381Suite()

	go listenForNewPubKey(txOut)
	go listenForStartSubmitGeneralKeyShare(txOut2)

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

func listenForStartSubmitGeneralKeyShare(txOut <-chan coretypes.ResultEvent) {
	for {
		select {
		case result := <-txOut:
			id, found := result.Events["start-send-general-keyshare.start-send-general-keyshare-identity"]
			if !found {
				continue
			}

			if len(id) < 1 {
				continue
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
	}
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
				log.Printf("nowClient: %d", nowClient.CurrentShare.Index)

				// newShare, index, err := nowClient.ShareApiClient.GetShare(getNowStr())
				hexShare := "29c861be5016b20f5a4397795e3f086d818b11ad02e0dd8ee28e485988b6cb07"
				shareByte, _ := hex.DecodeString(hexShare)

				parsedShare := bls.NewKyberScalar()
				err = parsedShare.UnmarshalBinary(shareByte)

				var shareIndex uint64 = 1

				var share = &distIBE.Share{
					Index: bls.NewKyberScalar().SetInt64(int64(1)),
					Value: parsedShare,
				}

				if err != nil {
					log.Printf("[%d] Error getting the pending keyshare: %s", nowI, err.Error())
					return
				}
				validatorCosmosClients[nowI].SetPendingShare(&KeyShare{
					Share: *share,
					Index: shareIndex,
				})
				validatorCosmosClients[nowI].SetPendingShareExpiryBlock(expiryHeight)
				log.Printf("Got [%d] Client's New Share: %v | Expires at: %d\n", nowI, share.Value, expiryHeight)
			}
		}
	}
}
