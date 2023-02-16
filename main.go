package main

import (
	distIBE "DistributedIBE"
	"context"
	"encoding/hex"
	fbTypes "fairyring/x/fairblock/types"
	"fairyring/x/fairyring/types"
	"fmt"
	bls "github.com/drand/kyber-bls12381"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
	"github.com/tendermint/tendermint/types/time"
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

var ValidatorNameList = []string{"alice"} // , "bob"}
var TotalValidatorNumber = len(ValidatorNameList)

const Threshold = 1
const IBEId = "Random_IBE_ID"

const AddressPrefix = "cosmos"
const AuctionAddressPrefix = "auction"

func main() {
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

	validatorAccountList := make([]cosmosaccount.Account, TotalValidatorNumber)
	validatorAddressList := make([]string, TotalValidatorNumber)
	for i, eachAccountName := range ValidatorNameList {
		account, err := cosmos.Account(eachAccountName)
		if err != nil {
			log.Fatal(err)
		}
		validatorAccountList[i] = account

		addr, err := account.Address(AddressPrefix)
		if err != nil {
			log.Fatal(err)
		}
		validatorAddressList[i] = addr
		log.Printf("%s's address: %s\n", eachAccountName, addr)

		msg := &types.MsgRegisterValidator{
			Creator: addr,
		}

		_, err = cosmos.BroadcastTx(context.Background(), account, msg)
		if err != nil {
			if !strings.Contains(err.Error(), "validator already registered") {
				log.Fatal(err)
			}
		}

		log.Printf("%s's Registered as Validator", eachAccountName)
	}

	query := "tm.event = 'NewBlockHeader'"
	out, err := client.Subscribe(context.Background(), "", query)
	if err != nil {
		log.Fatal(err)
	}

	defer client.Stop()

	// Setup
	s := bls.NewBLS12381Suite()
	var secretVal []byte = []byte{187}
	var qBig = distIBE.BigFromHex("0x73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001")

	secret, _ := distIBE.H3(s, secretVal, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	publicKey := s.G1().Point().Mul(secret, s.G1().Point().Base())

	for {
		select {
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			fmt.Println("")
			log.Println("Got new block height: ", height)

			// Generate a new secret every 25 blocks
			if height%25 == 0 {
				secret, _ = distIBE.H3(s, secretVal, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
				publicKey = s.G1().Point().Mul(secret, s.G1().Point().Base())
			}

			// generating secret shares
			shares, _ := distIBE.GenerateShares(uint32(TotalValidatorNumber), uint32(Threshold), secret, qBig)

			// Public Key
			publicKeyBytes, _ := publicKey.MarshalBinary()
			publicKeyHex := hex.EncodeToString(publicKeyBytes)

			log.Println("Public Key: ", publicKeyHex)
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
			var c []distIBE.Commitment
			for j := 0; j < TotalValidatorNumber; j++ {
				c = append(c, distIBE.Commitment{
					Sp: s.G1().Point().Mul(
						shares[j].Value,
						s.G1().Point().Base(),
					),
					Index: uint32(j + 1),
				})
			}

			// Extracting the keys using shares
			var sk []distIBE.ExtractedKey
			for k := 0; k < TotalValidatorNumber; k++ {
				sk = append(sk, distIBE.Extract(s, shares[k].Value, uint32(k+1), []byte(IBEId)))
			}

			aggregated, _ := distIBE.AggregateSK(s, sk, c, []byte(IBEId))

			aggregatedBytes, err := aggregated.MarshalBinary()
			if err != nil {
				log.Fatal(err)
			}

			hexAggregated := hex.EncodeToString(aggregatedBytes)

			broadcastMsg := &fbTypes.MsgCreateAggregatedKeyShare{
				Creator:   auctionAliceAddress,
				Data:      hexAggregated,
				Height:    uint64(height),
				PublicKey: publicKeyHex,
			}

			_, err = auctionCosmos.BroadcastTx(
				context.Background(),
				auctionAlice,
				broadcastMsg,
			)

			if err != nil {
				log.Fatal(err)
			}

			log.Printf("Height: %d, Submitted: %s\n", uint64(height), hexAggregated)

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
