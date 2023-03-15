package main

import (
	distIBE "DistributedIBE"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fairyring/x/fairyring/types"
	"fairyringclient/cosmosClient"
	"fairyringclient/shareAPIClient"
	"fmt"
	bls "github.com/drand/kyber-bls12381"
	"github.com/joho/godotenv"
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

func setupShareClient(pks []string, totalValidatorNum uint64, managerPrivateKey, apiUrl string) (string, error) {
	shareClient, err := shareAPIClient.NewShareAPIClient(apiUrl, managerPrivateKey)
	if err != nil {
		return "", err
	}

	threshold := uint64(math.Ceil(float64(totalValidatorNum) * (2.0 / 3.0)))

	result, err := shareClient.Setup(totalValidatorNum, threshold, pks)
	if err != nil {
		return "", err
	}

	return result.MPK, nil
}

func main() {
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
	PrivateKeyFile := os.Getenv("PRIVATE_KEY_FILE")
	PubKeyFileNamePrefix := os.Getenv("PUB_KEY_FILE_NAME_PREFIX")
	PubKeyFileNameFormat := os.Getenv("PUB_KEY_FILE_NAME_FORMAT")

	ApiUrl := os.Getenv("API_URL")

	PrivateKey := os.Getenv("VALIDATOR_PRIVATE_KEY")

	myCosmosClient, err := cosmosClient.NewCosmosClient(
		fmt.Sprintf("%s:%s", gRPCIP, gRPCPort),
		PrivateKey,
		"fairyring",
	)
	if err != nil {
		log.Fatal("Error creating custom cosmos client: ", err)
	}

	addr := myCosmosClient.GetAddress()
	log.Printf("Cosmos Client Loaded Address: %s\n", addr)

	var masterPublicKey string

	shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, PrivateKeyFile)
	if err != nil {
		log.Fatal(err)
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

		_masterPublicKey, err := setupShareClient(pks, TotalValidatorNum, ManagerPrivateKey, ApiUrl)
		if err != nil {
			log.Fatal(err)
		}
		masterPublicKey = _masterPublicKey
	} else {
		_masterPublicKey, err := shareClient.GetMasterPublicKey()

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

	_, err = myCosmosClient.BroadcastTx(&types.MsgRegisterValidator{
		Creator: addr,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "validator already registered") {
			log.Fatal(err)
		}
	}

	log.Printf("%s Registered as Validator", addr)

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
		_, err := myCosmosClient.BroadcastTx(
			&types.MsgCreateLatestPubKey{
				Creator:   addr,
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

			share, index, err := shareClient.GetShare(processHeightStr)
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

			log.Printf("Got Share for height %s took: %d ms\n", processHeightStr, gotShareTookTime.Milliseconds())

			go func() {
				_, err = myCosmosClient.BroadcastTx(&types.MsgSendKeyshare{
					Creator:       addr,
					Message:       extractedKeyHex,
					Commitment:    hex.EncodeToString(commitmentBinary),
					KeyShareIndex: index,
					BlockHeight:   processHeight,
				})
				if err != nil {
					log.Printf("Submit KeyShare for Height %s ERROR: %s | Took: %.1f s\n", processHeightStr, err.Error(), time.Since(gotShareTime).Seconds())
				}

				log.Printf("Submit KeyShare for Height %s Confirmed | Took: %.1f s\n", processHeightStr, time.Since(gotShareTime).Seconds())
			}()
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
