package main2

import (
	"context"
	"fairyring/x/fairyring/types"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/ignite/cli/ignite/pkg/cosmosclient"

	tmclient "github.com/tendermint/tendermint/rpc/client/http"
	tmtypes "github.com/tendermint/tendermint/types"
)

var (
	done      chan interface{}
	interrupt chan os.Signal
)

func main() {
	addressPrefix := "cosmos"

	cosmos, err := cosmosclient.New(
		context.Background(),
		cosmosclient.WithAddressPrefix(addressPrefix),
	)
	if err != nil {
		log.Fatal(err)
	}

	accountName := "alice"

	account, err := cosmos.Account(accountName)
	if err != nil {
		log.Fatal(err)
	}

	addr, err := account.Address(addressPrefix)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Address: ", addr)

	msg := &types.MsgRegisterValidator{
		Creator: addr,
	}

	txResp, err := cosmos.BroadcastTx(context.Background(), account, msg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("RegisterValidator response: ", txResp)

	client, err := tmclient.New("http://localhost:26657", "/websocket")
	err = client.Start()
	if err != nil {
		log.Fatal(err)
	}

	query := "tm.event = 'NewBlockHeader'"
	out, err := client.Subscribe(context.Background(), "", query)
	if err != nil {
		log.Fatal(err)
	}

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	defer client.Stop()

	for {
		select {
		case <-quit:
			log.Println("Terminate, closing all connections")
			err = client.UnsubscribeAll(context.Background(), query)
			if err != nil {
				log.Println("Error closing websocket: ", err)
			}
			os.Exit(0)
		case result := <-out:
			height := result.Data.(tmtypes.EventDataNewBlockHeader).Header.Height
			log.Println("got new block", "height", height)
			log.Println("sending next height")
			broadcastMsg := &types.MsgSendKeyshare{
				Creator:     addr,
				Message:     fmt.Sprintf("alice keyshare submitted %d", height),
				BlockHeight: uint64(height) + 1,
			}
			broadcastResp, err := cosmos.BroadcastTx(context.Background(), account, broadcastMsg)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Sent keyshare: ", broadcastResp)
		}
	}
}
