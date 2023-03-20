package main

import (
	distIBE "DistributedIBE"
	"context"
	cosmosmath "cosmossdk.io/math"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fairyring/x/fairyring/types"
	"fairyringclient/cosmosClient"
	"fairyringclient/shareAPIClient"
	"fmt"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	bls "github.com/drand/kyber-bls12381"
	"github.com/joho/godotenv"
	"io"
	"log"
	"math"
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

func setupShareClient(shareClient shareAPIClient.ShareAPIClient, pks []string, totalValidatorNum uint64) (string, error) {
	threshold := uint64(math.Ceil(float64(totalValidatorNum) * (2.0 / 3.0)))

	result, err := shareClient.Setup(totalValidatorNum, threshold, pks)
	if err != nil {
		return "", err
	}

	return result.MPK, nil
}

type ValidatorClients struct {
	CosmosClient   *cosmosClient.CosmosClient
	ShareApiClient *shareAPIClient.ShareAPIClient
}

func main() {
	var masterCosmosClient ValidatorClients
	var validatorCosmosClients []ValidatorClients

	// get all the variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	NodeIP := os.Getenv("NODE_IP_ADDRESS")
	NodePort := os.Getenv("NODE_PORT")

	gRPCIP := os.Getenv("GRPC_IP_ADDRESS")
	gRPCPort := os.Getenv("GRPC_PORT")

	TotalValidatorNumStr := os.Getenv("TOTAL_VALIDATOR_NUM")
	TotalValidatorNum, err := strconv.ParseUint(TotalValidatorNumStr, 10, 64)
	if err != nil {
		log.Fatal("Error parsing total validator num from .env")
	}

	IsManagerStr := os.Getenv("IS_MANAGER")
	isManager, err := strconv.ParseBool(IsManagerStr)
	if err != nil {
		log.Fatal("Error parsing isManager from .env")
	}

	ManagerPrivateKey := os.Getenv("MANAGER_PRIVATE_KEY")
	PrivateKeyFileNamePrefix := os.Getenv("PRIVATE_KEY_FILE_NAME_PREFIX")
	PubKeyFileNamePrefix := os.Getenv("PUB_KEY_FILE_NAME_PREFIX")

	ApiUrl := os.Getenv("API_URL")

	// Import Master Private Key & Create Clients
	MasterPrivateKey := os.Getenv("MASTER_PRIVATE_KEY")
	if len(MasterPrivateKey) > 1 {
		masterClient, err := cosmosClient.NewCosmosClient(
			fmt.Sprintf("%s:%s", gRPCIP, gRPCPort),
			MasterPrivateKey,
			"fairyring",
		)
		if err != nil {
			log.Fatal("Error creating custom cosmos client: ", err)
		}

		addr := masterClient.GetAddress()
		bal, err := masterClient.GetBalance("frt")
		if err != nil {
			log.Fatal("Error getting master account balance: ", err)
		}
		log.Printf("Master Cosmos Client Loaded Address: %s , Balance: %s FRT\n", addr, bal.String())

		shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, ManagerPrivateKey)
		if err != nil {
			log.Fatal("Error creating share api client:", err)
		}
		masterCosmosClient = ValidatorClients{
			CosmosClient:   masterClient,
			ShareApiClient: shareClient,
		}
	}

	allPrivateKeys, err := readPrivateKeysJsonFile("privateKeys.json")
	if err != nil {
		log.Fatal("Error loading private keys json: ", err)
	}

	validatorCosmosClients = make([]ValidatorClients, len(allPrivateKeys))

	log.Println("Loading total:", len(allPrivateKeys), "private key(s)")

	allAccAddrs := make([]cosmostypes.AccAddress, len(allPrivateKeys))

	for index, eachPKey := range allPrivateKeys {
		eachClient, err := cosmosClient.NewCosmosClient(
			fmt.Sprintf("%s:%s", gRPCIP, gRPCPort),
			eachPKey,
			"fairyring",
		)
		if err != nil {
			accAddr, err := cosmosClient.PrivateKeyToAccAddress(eachPKey)
			if err != nil {
				log.Fatal("Error extract address from private key: ", err)
			}
			resp, err := masterCosmosClient.CosmosClient.SendToken(accAddr.String(), "frt", cosmosmath.NewInt(10))
			if err != nil {
				log.Fatal("Error activating account: ", accAddr.String(), ": ", err)
			}

			log.Println("Successfully activate account: ", accAddr.String(), "\n", resp)
			_eachClient, err := cosmosClient.NewCosmosClient(
				fmt.Sprintf("%s:%s", gRPCIP, gRPCPort),
				eachPKey,
				"fairyring",
			)
			if err != nil {
				log.Fatal("Error creating custom cosmos client: ", err)
			}
			eachClient = _eachClient
		} else {
			addr := eachClient.GetAddress()
			log.Printf("Validator Cosmos Client Loaded Address: %s\n", addr)
		}

		shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, fmt.Sprintf("%s%d%s", PrivateKeyFileNamePrefix, index+1, PrivateKeyFileNameFormat))
		if err != nil {
			log.Fatal("Error creating share api client:", err)
		}

		bal, err := eachClient.GetBalance("frt")
		if err != nil {
			log.Fatal("Error getting", eachClient.GetAddress(), "account balance: ", err)
		}
		log.Printf("Address: %s , Balance: %s FRT\n", eachClient.GetAddress(), bal.String())

		validatorCosmosClients[index] = ValidatorClients{
			CosmosClient:   eachClient,
			ShareApiClient: shareClient,
		}

		allAccAddrs[index] = eachClient.GetAccAddress()
	}

	var masterPublicKey string

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

		_masterPublicKey, err := setupShareClient(*masterCosmosClient.ShareApiClient, pks, TotalValidatorNum)
		if err != nil {
			log.Fatal(err)
		}
		masterPublicKey = _masterPublicKey
	} else {
		_masterPublicKey, err := validatorCosmosClients[0].ShareApiClient.GetMasterPublicKey()

		if err != nil {
			log.Fatal(err)
		}
		masterPublicKey = _masterPublicKey
	}

	client, err := tmclient.New(fmt.Sprintf("%s:%s", NodeIP, NodePort), "/websocket")
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	for i, eachClient := range validatorCosmosClients {
		eachAddr := eachClient.CosmosClient.GetAddress()
		_, err = eachClient.CosmosClient.BroadcastTx(&types.MsgRegisterValidator{
			Creator: eachAddr,
		})
		if err != nil {
			if !strings.Contains(err.Error(), "validator already registered") {
				log.Fatal(err)
			}
		}
		log.Printf("%d. %s Registered as Validator", i, eachAddr)
	}

	query := "tm.event = 'NewBlockHeader'"
	out, err := client.Subscribe(context.Background(), "", query)
	if err != nil {
		log.Fatal(err)
	}

	defer client.Stop()

	s := bls.NewBLS12381Suite()

	decodedPublicKey, err := base64.StdEncoding.DecodeString(masterPublicKey)
	if err != nil {
		log.Fatal(err)
	}
	publicKeyInHex := hex.EncodeToString(decodedPublicKey)

	// Submit the pubkey & id to fairyring
	if isManager {
		_, err := masterCosmosClient.CosmosClient.BroadcastTx(
			&types.MsgCreateLatestPubKey{
				Creator:   masterCosmosClient.CosmosClient.GetAddress(),
				PublicKey: publicKeyInHex,
			},
		)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Manager Submitted latest public key")
	}

	for {
		select {
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			fmt.Println("")

			processHeight := uint64(height + 1)
			processHeightStr := strconv.FormatUint(processHeight, 10)

			newHeightTime := time.Now()
			log.Printf("Latest Block Height: %d | Getting Share for Block: %s\n", height, processHeightStr)

			for i, each := range validatorCosmosClients {
				share, index, err := each.ShareApiClient.GetShare(processHeightStr)
				if err != nil {
					log.Fatal(err)
				}

				gotShareTookTime := time.Since(newHeightTime)
				gotShareTime := time.Now()

				extractedKey := distIBE.Extract(s, share.Value, uint32(index), []byte(processHeightStr))
				extractedKeyBinary, err := extractedKey.Sk.MarshalBinary()
				if err != nil {
					log.Fatal(err)
				}
				extractedKeyHex := hex.EncodeToString(extractedKeyBinary)

				commitmentPoint := s.G1().Point().Mul(share.Value, s.G1().Point().Base())
				commitmentBinary, err := commitmentPoint.MarshalBinary()

				if err != nil {
					log.Fatal(err)
				}

				log.Printf("[%d] Got Share for height %s took: %d ms\n", i, processHeightStr, gotShareTookTime.Milliseconds())

				go func() {
					_, err = each.CosmosClient.BroadcastTx(&types.MsgSendKeyshare{
						Creator:       each.CosmosClient.GetAddress(),
						Message:       extractedKeyHex,
						Commitment:    hex.EncodeToString(commitmentBinary),
						KeyShareIndex: index,
						BlockHeight:   processHeight,
					})
					if err != nil {
						log.Printf("[%d] Submit KeyShare for Height %s ERROR: %s | Took: %.1f s\n", i, processHeightStr, err.Error(), time.Since(gotShareTime).Seconds())
					}

					log.Printf("[%d] Submit KeyShare for Height %s Confirmed | Took: %.1f s\n", i, processHeightStr, time.Since(gotShareTime).Seconds())
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

func readPrivateKeysJsonFile(filePath string) ([]string, error) {
	jsonFile, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	defer jsonFile.Close()

	var keys []string

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(byteValue, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
