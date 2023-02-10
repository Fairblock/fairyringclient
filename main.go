package main

import (
	distIBE "DistributedIBE"
	"context"
	"encoding/hex"
	"fairyring/x/fairyring/types"
	bls "github.com/drand/kyber-bls12381"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/ignite/pkg/cosmosclient"
	"log"
	"os"
	"strings"

	tmclient "github.com/tendermint/tendermint/rpc/client/http"
	tmtypes "github.com/tendermint/tendermint/types"
)

var (
	done      chan interface{}
	interrupt chan os.Signal
)

var ValidatorNameList = []string{"alice"} //, "bob"}
var TotalValidatorNumber = len(ValidatorNameList)

const Threshold = 1
const IBEId = "Random_IBE_ID"

const AddressPrefix = "cosmos"

func main() {
	// Create the cosmos client
	cosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(AddressPrefix),
	)
	if err != nil {
		log.Fatal(err)
	}

	client, err := tmclient.New("http://localhost:26657", "/websocket")
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	validatorAccountList := make([]cosmosaccount.Account, TotalValidatorNumber)
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

	//quit := make(chan os.Signal)
	//signal.Notify(quit, os.Interrupt)

	defer client.Stop()

	// Setup
	s := bls.NewBLS12381Suite()
	var secretVal []byte = []byte{187}
	var qBig = distIBE.BigFromHex("0x73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001")
	secret, _ := distIBE.H3(s, secretVal, []byte("This is the secret message"))

	for {
		select {
		//case <-quit:
		//	log.Println("Terminate, closing all connections")
		//	err = client.UnsubscribeAll(context.Background(), query)
		//	if err != nil {
		//		log.Println("Error closing websocket: ", err)
		//	}
		//	os.Exit(0)
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			log.Println("Got new block height: ", height)

			// generating secret shares
			shares, _ := distIBE.GenerateShares(uint32(TotalValidatorNumber), uint32(Threshold), secret, qBig)

			// Public Key
			// PK := s.G1().Point().Mul(secret, s.G1().Point().Base())

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

			for i, eachValidatorAccount := range validatorAccountList {
				eachAddress, err := eachValidatorAccount.Address(AddressPrefix)
				if err != nil {
					log.Fatal(err)
				}

				out, err := sk[i].GetSk().MarshalBinary()
				if err != nil {
					log.Fatal(err)
				}

				cOut, err := c[i].Sp.MarshalBinary()
				if err != nil {
					log.Fatal(err)
				}

				hexKey := hex.EncodeToString(out)
				hexCommitment := hex.EncodeToString(cOut)

				broadcastMsg := &types.MsgSendKeyshare{
					Creator:       eachAddress,
					Message:       hexKey,
					Commitment:    hexCommitment,
					KeyShareIndex: uint64(sk[i].GetIndex()),
					BlockHeight:   uint64(height) + 2,
				}
				log.Printf("Broadcasting")
				_, err = cosmos.BroadcastTx(context.Background(), eachValidatorAccount, broadcastMsg)
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("\nSent KeyShare at Block Height: %d\nKey: %s\nCommitment: %s\nKey Index: %d Commitment Index: %d\n", height, hexKey, hexCommitment, sk[i].GetIndex(), c[i].Index)
			}
		}
	}
}
