package main

import (
	"context"
	"fairyring/x/fairyring/types"
	"fairyringclient/shareAPIClient"
	"fmt"
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

const ApiUrl = "https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com"
const ManagerPrivateKey = "keys/skManager.pem"
const PrivateKeyFile = "keys/sk1.pem"
const PubKeyFile1 = "keys/pk1.pem"
const PubKeyFile2 = "keys/pk2.pem"
const Msg = "random_msg"

const ValidatorName = "alice"
const TotalValidatorNum = 2
const Threshold = 1

const AddressPrefix = "cosmos"
const AuctionAddressPrefix = "auction"

func setupShareClient(pks []string) (string, error) {
	shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, ManagerPrivateKey)
	if err != nil {
		return "", err
	}

	result, err := shareClient.Setup(TotalValidatorNum, Threshold, Msg, pks)
	if err != nil {
		return "", err
	}

	return result, nil
}

func main() {

	pk1, err := readPemFile(PubKeyFile1)
	if err != nil {
		log.Fatal(err)
	}

	pk2, err := readPemFile(PubKeyFile2)
	if err != nil {
		log.Fatal(err)
	}

	masterPublicKey, err := setupShareClient([]string{pk1, pk2})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Setup Result: %s", masterPublicKey)

	shareClient, err := shareAPIClient.NewShareAPIClient(ApiUrl, PrivateKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	// Create the cosmos client
	cosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(AddressPrefix),
	)
	if err != nil {
		log.Fatal(err)
	}

	auctionCosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(AuctionAddressPrefix),
		cosmosclient.WithHome("~/.destination_auction/"),
		cosmosclient.WithNodeAddress("tcp://localhost:26659"),
	)
	if err != nil {
		log.Fatal(err)
	}

	client, err := tmclient.New("http://localhost:26657", "/websocket")
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	auctionAlice, err := auctionCosmos.Account("bob")
	if err != nil {
		log.Fatal(err)
	}
	auctionAliceAddress, err := auctionAlice.Address(AuctionAddressPrefix)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("\nGot Auction Alice Address %s", auctionAliceAddress)

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

	for {
		select {
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			fmt.Println("")
			log.Println("Got new block height: ", height)

			heightInStr := strconv.FormatInt(height, 10)

			share, err := shareClient.GetShare(heightInStr)
			if err != nil {
				log.Fatal(err)
			}

			log.Printf("Share for height %s: %s", heightInStr, share.EncShare)
			// generating secret shares

			// Submit the pubkey & id to fairyring
			//_, err := cosmos.BroadcastTx(
			//	context.Background(),
			//	validatorAccountList[0],
			//	&types.MsgCreatePubKeyID{
			//		Creator:   validatorAddressList[0],
			//		Height:    uint64(height),
			//		PublicKey: publicKeyHex,
			//		IbeID:     IBEId,
			//	},
			//)
			//if err != nil {
			//	log.Fatal(err)
			//}
			//log.Println("Submitted PubKey & ID for block: ", strconv.FormatInt(height, 10))

			// Generating commitments
			//var c []distIBE.Commitment
			//for j := 0; j < TotalValidatorNum; j++ {
			//	c = append(c, distIBE.Commitment{
			//		Sp: s.G1().Point().Mul(
			//			shares[j].Value,
			//			s.G1().Point().Base(),
			//		),
			//		Index: uint32(j + 1),
			//	})
			//}
			//
			//// Extracting the keys using shares
			//var sk []distIBE.ExtractedKey
			//for k := 0; k < TotalValidatorNum; k++ {
			//	sk = append(sk, distIBE.Extract(s, shares[k].Value, uint32(k+1), []byte(heightInStr)))
			//}
			//
			//aggregated, _ := distIBE.AggregateSK(s, sk, c, []byte(heightInStr))
			//
			//aggregatedBytes, err := aggregated.MarshalBinary()
			//if err != nil {
			//	log.Fatal(err)
			//}
			//
			//hexAggregated := hex.EncodeToString(aggregatedBytes)
			//
			//broadcastMsg := &fbTypes.MsgCreateAggregatedKeyShare{
			//	Creator:   auctionAliceAddress,
			//	Data:      hexAggregated,
			//	Height:    uint64(height),
			//	PublicKey: publicKeyHex,
			//}
			//
			//_, err = auctionCosmos.BroadcastTx(
			//	context.Background(),
			//	auctionAlice,
			//	broadcastMsg,
			//)
			//
			//if err != nil {
			//	log.Fatal(err)
			//}
			//
			//log.Printf("Height: %d, Submitted: %s\n", uint64(height), hexAggregated)

			// --

			//for i, eachValidatorAccount := range validatorAccountList {
			//	eachAddress, err := eachValidatorAccount.Address(AddressPrefix)
			//	if err != nil {
			//		log.Fatal(err)
			//	}
			//
			//	out, err := sk[i].Sk.MarshalBinary()
			//	if err != nil {
			//		log.Fatal(err)
			//	}
			//
			//	cOut, err := c[i].Sp.MarshalBinary()
			//	if err != nil {
			//		log.Fatal(err)
			//	}
			//
			//	hexKey := hex.EncodeToString(out)
			//	hexCommitment := hex.EncodeToString(cOut)

			//broadcastMsg := &types.MsgSendKeyshare{
			//	Creator:       eachAddress,
			//	Message:       hexKey,
			//	Commitment:    hexCommitment,
			//	KeyShareIndex: uint64(sk[i].Index),
			//	BlockHeight:   uint64(height) + 1,
			//}
			//// log.Printf("Broadcasting")
			//_, err = cosmos.BroadcastTx(context.Background(), eachValidatorAccount, broadcastMsg)
			//if err != nil {
			//	log.Fatal(err)
			//}
			//log.Printf("\nSent KeyShare at Block Height: %d\nKey: %s\nCommitment: %s\nKey Index: %d Commitment Index: %d\n", height, hexKey, hexCommitment, sk[i].Index, c[i].Index)
			// }
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
