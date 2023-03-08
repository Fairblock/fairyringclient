package main

import (
	distIBE "DistributedIBE"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fairyring/x/fairyring/types"
	"fairyringclient/shareAPIClient"
	"fmt"
	bls "github.com/drand/kyber-bls12381"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
	"log"
	"os"
	"strconv"
	"strings"

	tmclient "github.com/tendermint/tendermint/rpc/client/http"
	tmtypes "github.com/tendermint/tendermint/types"
)

var (
	done      chan interface{}
	interrupt chan os.Signal
)

const NodeIP = "http://172.17.0.2"
const NodePort = "26657"
const ApiUrl = "https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com"
const ManagerPrivateKey = "keys/skManager.pem"
const PrivateKeyFile = "keys/sk1.pem"
const PubKeyFileNamePrefix = "keys/pk"
const PubKeyFileNameFormat = ".pem"

const ValidatorName = "validator_account"
const TotalValidatorNum = 3
const Threshold = 2
const isManager = true

const AddressPrefix = "cosmos"

func setupShareClient(pks []string) (string, error) {
	shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, ManagerPrivateKey)
	if err != nil {
		return "", err
	}

	result, err := shareClient.Setup(TotalValidatorNum, Threshold, pks)
	if err != nil {
		return "", err
	}
	replacedResult := strings.ReplaceAll(result, `"`, "")

	return replacedResult, nil
}

func main() {
	var masterPublicKey string

	shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, PrivateKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	if isManager {
		pks := make([]string, TotalValidatorNum)

		for i := 0; i < TotalValidatorNum; i++ {
			pk, err := readPemFile(fmt.Sprintf("%s%d%s", PubKeyFileNamePrefix, i+1, PubKeyFileNameFormat))
			if err != nil {
				log.Fatal(err)
			}

			pks[i] = pk
		}

		_masterPublicKey, err := setupShareClient(pks)
		if err != nil {
			log.Fatal(err)
		}
		masterPublicKey = _masterPublicKey
		log.Printf("Setup Result: %s", masterPublicKey)
	} else {
		_masterPublicKey, err := shareClient.GetMasterPublicKey()

		if err != nil {
			log.Fatal(err)
		}
		masterPublicKey = _masterPublicKey
		log.Printf("Got Master Public Key: %s", masterPublicKey)
	}

	// Create the cosmos client
	cosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(AddressPrefix),
		cosmosclient.WithNodeAddress(fmt.Sprintf("%s:%s", NodeIP, NodePort)),
	)
	if err != nil {
		log.Fatal(err)
	}

	client, err := tmclient.New(fmt.Sprintf("%s:%s", NodeIP, NodePort), "/websocket")
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	account, err := cosmos.Account(ValidatorName)
	if err != nil {
		log.Fatal(err)
	}
	addr, err := account.Address(AddressPrefix)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s's address: %s\n", ValidatorName, addr)

	msg := &types.MsgRegisterValidator{
		Creator: addr,
	}

	_, err = cosmos.BroadcastTx(context.Background(), account, msg)
	if err != nil {
		if !strings.Contains(err.Error(), "validator already registered") {
			log.Fatal(err)
		}
	}

	log.Printf("%s's Registered as Validator", ValidatorName)

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

	log.Printf("Public key in Hex: %s", publicKeyInHex)

	for {
		select {
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			fmt.Println("")
			log.Println("Got new block height: ", height)

			processHeight := uint64(height + 1)
			processHeightStr := strconv.FormatUint(processHeight, 10)

			// Submit the pubkey & id to fairyring
			go func() {
				_, err := cosmos.BroadcastTx(
					context.Background(),
					account,
					&types.MsgCreatePubKeyID{
						Creator:   addr,
						Height:    processHeight,
						PublicKey: publicKeyInHex,
						IbeID:     processHeightStr,
					},
				)
				if err != nil {
					log.Fatal(err)
				}
				log.Println("Submitted PubKey & ID for block: ", processHeightStr)
			}()

			share, index, err := shareClient.GetShare(processHeightStr)
			if err != nil {
				log.Fatal(err)
			}

			extractedKey := distIBE.Extract(s, share.Value, uint32(index), []byte(processHeightStr))
			extractedKeyBinary, err := extractedKey.Sk.MarshalBinary()
			if err != nil {
				log.Fatal(err)
			}
			extractedKeyHex := hex.EncodeToString(extractedKeyBinary)

			log.Printf("Share for height %s: %s", processHeightStr, extractedKeyHex)

			commitmentPoint := s.G1().Point().Mul(share.Value, s.G1().Point().Base())
			commitmentBinary, err := commitmentPoint.MarshalBinary()

			if err != nil {
				log.Fatal(err)
			}

			broadcastMsg := &types.MsgSendKeyshare{
				Creator:       addr,
				Message:       extractedKeyHex,
				Commitment:    hex.EncodeToString(commitmentBinary),
				KeyShareIndex: index,
				BlockHeight:   processHeight,
			}
			// log.Printf("Broadcasting")
			_, err = cosmos.BroadcastTx(context.Background(), account, broadcastMsg)
			if err != nil {
				log.Fatal(err)
			}

			log.Printf("Sent KeyShare at Block Height: %d\n", processHeight)
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
