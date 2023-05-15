package main

import (
	distIBE "DistributedIBE"
	"context"
	cosmosmath "cosmossdk.io/math"
	"encoding/hex"
	"fairyring/x/keyshare/types"
	"fairyringclient/cosmosClient"
	"fairyringclient/shareAPIClient"
	"fmt"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	bls "github.com/drand/kyber-bls12381"
	"github.com/joho/godotenv"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tmclient "github.com/tendermint/tendermint/rpc/client/http"
	tmtypes "github.com/tendermint/tendermint/types"
)

var (
	done      chan interface{}
	interrupt chan os.Signal
)

const PubKeyFileNameFormat = ".pem"
const PrivateKeyFileNameFormat = ".pem"

const ManagerPrivateKey = "keys/skManager.pem"
const PrivateKeyFileNamePrefix = "keys/sk"
const PubKeyFileNamePrefix = "keys/pk"

const APIUrl = "https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com"

const DefaultDenom = "frt"
const DefaultChainID = "fairyring"

var (
	masterCosmosClient     ValidatorClients
	validatorCosmosClients []ValidatorClients
)

func main() {

	// get all the variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	Denom := os.Getenv("DENOM")

	if len(Denom) == 0 {
		log.Println("DENOM not found in .env, using default denom: ", DefaultDenom)
		Denom = DefaultDenom
	}

	IsManagerStr := os.Getenv("IS_MANAGER")
	isManager, err := strconv.ParseBool(IsManagerStr)
	if err != nil {
		log.Fatal("Error parsing isManager from .env")
	}

	TotalValidatorNum, err := strconv.ParseUint(os.Getenv("TOTAL_VALIDATOR_NUM"), 10, 64)
	if err != nil && isManager {
		log.Fatal("Error parsing total validator num from .env, TOTAL_VALIDATOR_NUM is required for manager to setup")
	}

	setupGRPCIP := os.Getenv("SETUP_GRPC_IP_ADDRESS")
	gRPCIP := os.Getenv("GRPC_IP_ADDRESS")
	gRPCPort := os.Getenv("GRPC_PORT")
	setupGRpcEndpoint := fmt.Sprintf("%s:%s", setupGRPCIP, gRPCPort)
	gRPCEndpoint := fmt.Sprintf(
		"%s:%s",
		gRPCIP,
		gRPCPort,
	)

	// Import Master Private Key & Create Clients
	MasterPrivateKey := os.Getenv("MASTER_PRIVATE_KEY")
	if len(MasterPrivateKey) > 1 {
		masterClient, err := cosmosClient.NewCosmosClient(
			gRPCEndpoint,
			MasterPrivateKey,
			DefaultChainID,
		)
		if err != nil {
			log.Fatal("Error creating custom cosmos client: ", err)
		}

		addr := masterClient.GetAddress()
		bal, err := masterClient.GetBalance(Denom)
		if err != nil {
			log.Fatal("Error getting master account balance: ", err)
		}
		log.Printf("Master Cosmos Client Loaded Address: %s , Balance: %s %s\n", addr, bal.String(), Denom)

		shareClient, err := shareAPIClient.NewShareAPIClient(APIUrl, ManagerPrivateKey)
		if err != nil {
			log.Fatal("Error creating share api client:", err)
		}
		masterCosmosClient = ValidatorClients{
			CosmosClient:   masterClient,
			ShareApiClient: shareClient,
		}
		if isManager {
			pks := make([]string, TotalValidatorNum)
			var i uint64
			for i = 0; i < TotalValidatorNum; i++ {
				pk, err := readPemFile(fmt.Sprintf("%s%d%s", PubKeyFileNamePrefix, i+1, PubKeyFileNameFormat))
				if err != nil {
					log.Fatal(err)
				}

				pks[i] = pk
			}

			txHash, err := masterCosmosClient.SetupShareClient(
				setupGRpcEndpoint,
				DefaultChainID,
				pks,
				TotalValidatorNum,
			)
			if err != nil {
				log.Fatal("Error setting share client: ", err.Error())
			}
			log.Printf("\nSetup Tx Hash: %s\n", txHash)
		}
	}

	allPrivateKeys := strings.Split(os.Getenv("VALIDATOR_PRIVATE_KEYS"), ",")
	if len(allPrivateKeys) == 0 {
		log.Fatal("VALIDATOR_PRIVATE_KEYS not found")
	}

	validatorCosmosClients = make([]ValidatorClients, len(allPrivateKeys))

	log.Println("Loading total:", len(allPrivateKeys), "private key(s)")

	allAccAddrs := make([]cosmostypes.AccAddress, len(allPrivateKeys))

	privateKeyIndexStr := os.Getenv("PRIVATE_KEY_FILE_NAME_INDEX_START_AT")
	privateKeyIndexNum, err := strconv.ParseUint(privateKeyIndexStr, 10, 64)
	if err != nil {
		log.Println("Error loading private index, setting it to default value: 1")
		privateKeyIndexNum = 1
	}

	for index, eachPKey := range allPrivateKeys {
		eachClient, err := cosmosClient.NewCosmosClient(
			gRPCEndpoint,
			eachPKey,
			DefaultChainID,
		)
		if err != nil {
			accAddr, err := cosmosClient.PrivateKeyToAccAddress(eachPKey)
			if err != nil {
				log.Fatal("Error extract address from private key: ", err)
			}
			resp, err := masterCosmosClient.CosmosClient.SendToken(accAddr.String(), Denom, cosmosmath.NewInt(10), true)
			if err != nil {
				log.Fatal("Error activating account: ", accAddr.String(), ": ", err)
			}

			_, err = masterCosmosClient.CosmosClient.WaitForTx(resp.TxHash, 2*time.Second)
			if err != nil {
				log.Fatal("Error activating account: ", accAddr.String(), ": ", err)
			}

			log.Println("Successfully activate account: ", accAddr.String())

			_eachClient, err := cosmosClient.NewCosmosClient(
				gRPCEndpoint,
				eachPKey,
				DefaultChainID,
			)
			if err != nil {
				log.Fatal("Error creating custom cosmos client: ", err)
			}
			eachClient = _eachClient
		} else {
			addr := eachClient.GetAddress()
			log.Printf("Validator Cosmos Client Loaded Address: %s\n", addr)
		}

		shareClient, err := shareAPIClient.NewShareAPIClient(
			APIUrl,
			fmt.Sprintf(
				"%s%d%s",
				PrivateKeyFileNamePrefix,
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

		log.Printf("Successfully fetch pub keys on pep module: Active: %v | Queued: %v", pubKeys.ActivePubKey, pubKeys.QueuedPubKey)

		// Queued Pub key exists on pep module
		if len(pubKeys.QueuedPubKey.PublicKey) > 1 && pubKeys.QueuedPubKey.Expiry > 0 {
			previousShare, previousShareIndex, err := shareClient.GetLastShare(getNowStr())
			if err != nil {
				log.Fatal("Error getting previous share:", err)
			}
			log.Printf("Got previous share: %s | Index: %d", previousShare, previousShareIndex)

			if previousShare != nil {
				validatorCosmosClients[index].SetCurrentShare(&KeyShare{
					Share: *previousShare,
					Index: previousShareIndex,
				})
				validatorCosmosClients[index].SetPendingShare(&KeyShare{
					Share: *share,
					Index: shareIndex,
				})
				validatorCosmosClients[index].SetExpiryBlock(pubKeys.ActivePubKey.Expiry)
			}
		}
	}

	client, err := tmclient.New(
		fmt.Sprintf(
			"%s:%s",
			os.Getenv("NODE_IP_ADDRESS"),
			os.Getenv("NODE_PORT"),
		),
		"/websocket",
	)
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

			latestHeight, err := validatorCosmosClients[0].CosmosClient.GetLatestHeight()
			if err != nil {
				log.Printf("Error getting latest height from pep, setting latest height as 0...")
			}

			log.Printf("Latest Block Height: %d | Deriving Share for Height: %s\n", height, processHeightStr)

			for i, each := range validatorCosmosClients {
				nowI := i
				nowEach := each
				go func() {
					if nowEach.CurrentShareExpiryBlock != 0 && nowEach.CurrentShareExpiryBlock <= latestHeight {
						log.Printf("[%d] Latest Height: %d | Old share expiring, updating to new share\n", nowI, latestHeight)
						if nowEach.PendingShare == nil {
							log.Printf("Pending share not found for client no.%d\n", nowI)
							return
						}

						validatorCosmosClients[nowI].ActivatePendingShare()
					}
					currentShare := nowEach.CurrentShare

					extractedKey := distIBE.Extract(s, currentShare.Share.Value, uint32(currentShare.Index), []byte(processHeightStr))
					extractedKeyBinary, err := extractedKey.Sk.MarshalBinary()
					if err != nil {
						log.Fatal(err)
					}
					extractedKeyHex := hex.EncodeToString(extractedKeyBinary)

					commitmentPoint := s.G1().Point().Mul(currentShare.Share.Value, s.G1().Point().Base())
					commitmentBinary, err := commitmentPoint.MarshalBinary()

					if err != nil {
						log.Fatal(err)
					}

					go func() {
						resp, err := nowEach.CosmosClient.BroadcastTx(&types.MsgSendKeyshare{
							Creator:       nowEach.CosmosClient.GetAddress(),
							Message:       extractedKeyHex,
							Commitment:    hex.EncodeToString(commitmentBinary),
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

func listenForNewPubKey(txOut <-chan coretypes.ResultEvent) {
	for {
		select {
		case result := <-txOut:
			pubKey, found := result.Events["queued-pubkey-created.queued-pubkey-created-pubkey"]
			if !found {
				continue
			}

			expiryHeight, found := result.Events["queued-pubkey-created.queued-pubkey-created-expiry-height"]
			if !found {
				continue
			}

			activePubKeyExpiryHeightStr, found := result.Events["queued-pubkey-created.queued-pubkey-created-active-pubkey-expiry-height"]
			if !found {
				continue
			}
			activePubKeyExpiryHeight, err := strconv.ParseUint(activePubKeyExpiryHeightStr[0], 10, 64)
			if err != nil {
				log.Printf("Error parsing active pubkey expiry height: %s\n", err.Error())
				continue
			}

			log.Printf("\nNew Pubkey found: %s | Expiry Height: %s\n", pubKey, expiryHeight)

			for i, eachClient := range validatorCosmosClients {
				nowI := i
				nowClient := eachClient
				go func() {
					newShare, index, err := nowClient.ShareApiClient.GetShare(getNowStr())
					if err != nil {

					}
					validatorCosmosClients[nowI].SetPendingShare(&KeyShare{
						Share: *newShare,
						Index: index,
					})
					validatorCosmosClients[nowI].SetExpiryBlock(activePubKeyExpiryHeight)
					log.Printf("\nGot [%d] Client's New Share: %v\n", nowI, newShare.Value)
				}()
			}
		}
	}
}

func readPemFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}

	defer file.Close()

	//Create a byte slice (pemBytes) the size of the file size
	pemFileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}

	pemBytes := make([]byte, pemFileInfo.Size())
	file.Read(pemBytes)
	if err != nil {
		return "", err
	}

	return string(pemBytes), nil
}

func getNowStr() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
