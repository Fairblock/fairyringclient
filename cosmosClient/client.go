package cosmosClient

import (
	"context"
	"cosmossdk.io/math"
	"encoding/hex"
	"fairyring/app"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"log"
	"strings"
)

type CosmosClient struct {
	authClient      authtypes.QueryClient
	txClient        tx.ServiceClient
	bankQueryClient banktypes.QueryClient
	bankMsgClient   banktypes.MsgClient
	privateKey      secp256k1.PrivKey
	publicKey       cryptotypes.PubKey
	account         authtypes.BaseAccount
	accAddress      cosmostypes.AccAddress
	chainID         string
}

func NewCosmosClient(
	endpoint string,
	privateKeyHex string,
	chainID string,
) (*CosmosClient, error) {
	grpcConn, err := grpc.Dial(
		endpoint,
		grpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	authClient := authtypes.NewQueryClient(grpcConn)
	bankClient := banktypes.NewQueryClient(grpcConn)
	bankMsgClient := banktypes.NewMsgClient(grpcConn)

	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, err
	}

	privateKey := secp256k1.PrivKey{Key: keyBytes}
	pubKey := privateKey.PubKey()
	address := pubKey.Address()

	accAddr := cosmostypes.AccAddress(address)
	addr := accAddr.String()

	var baseAccount authtypes.BaseAccount

	resp, err := authClient.Account(
		context.Background(),
		&authtypes.QueryAccountRequest{Address: addr},
	)

	if err != nil {
		log.Println(cosmostypes.AccAddress(address).String())
		return nil, err
	}

	err = baseAccount.Unmarshal(resp.Account.Value)
	if err != nil {
		return nil, err
	}

	return &CosmosClient{
		bankQueryClient: bankClient,
		bankMsgClient:   bankMsgClient,
		authClient:      authClient,
		txClient:        tx.NewServiceClient(grpcConn),
		privateKey:      privateKey,
		account:         baseAccount,
		accAddress:      accAddr,
		publicKey:       pubKey,
		chainID:         chainID,
	}, nil
}

func (c *CosmosClient) GetBalance(denom string) (*math.Int, error) {
	resp, err := c.bankQueryClient.Balance(
		context.Background(),
		&banktypes.QueryBalanceRequest{
			Address: c.GetAddress(),
			Denom:   denom,
		},
	)
	if err != nil {
		return nil, err
	}
	return &resp.Balance.Amount, nil
}

func (c *CosmosClient) SendToken(target, denom string, amount math.Int) (*banktypes.MsgSendResponse, error) {
	resp, err := c.bankMsgClient.Send(
		context.Background(),
		&banktypes.MsgSend{
			FromAddress: c.GetAddress(),
			ToAddress:   target,
			Amount:      cosmostypes.NewCoins(cosmostypes.NewCoin(denom, amount)),
		},
	)
	return resp, err
}

func (c *CosmosClient) MultiSend(denom string, totalAmount, eachAmt math.Int, targets []cosmostypes.AccAddress) (*banktypes.MsgMultiSendResponse, error) {
	outputs := make([]banktypes.Output, len(targets))
	for i, each := range targets {
		outputs[i] = banktypes.NewOutput(each, cosmostypes.NewCoins(cosmostypes.NewCoin(denom, eachAmt)))
	}
	resp, err := c.bankMsgClient.MultiSend(
		context.Background(),
		&banktypes.MsgMultiSend{
			Inputs:  []banktypes.Input{banktypes.NewInput(c.accAddress, cosmostypes.NewCoins(cosmostypes.NewCoin(denom, totalAmount)))},
			Outputs: outputs,
		},
	)
	return resp, err
}

func (c *CosmosClient) GetAddress() string {
	return c.account.Address
}

func (c *CosmosClient) GetAccAddress() cosmostypes.AccAddress {
	return c.accAddress
}

func (c *CosmosClient) handleBroadcastResult(resp *cosmostypes.TxResponse, err error) error {
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errors.New("make sure that your account has enough balance")
		}
		return err
	}

	if resp.Code > 0 {
		return errors.Errorf("error code: '%d' msg: '%s'", resp.Code, resp.RawLog)
	}
	return nil
}

func (c *CosmosClient) BroadcastTx(msg cosmostypes.Msg) (*cosmostypes.TxResponse, error) {
	txBytes, err := c.signTxMsg(msg)
	if err != nil {
		return nil, err
	}

	c.account.Sequence++

	resp, err := c.txClient.BroadcastTx(
		context.Background(),
		&tx.BroadcastTxRequest{
			TxBytes: txBytes,
			Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
		},
	)
	if err != nil {
		return nil, err
	}

	return resp.TxResponse, c.handleBroadcastResult(resp.TxResponse, err)
}

func (c *CosmosClient) signTxMsg(msg cosmostypes.Msg) ([]byte, error) {
	encodingCfg := app.MakeEncodingConfig()
	txBuilder := encodingCfg.TxConfig.NewTxBuilder()
	signMode := encodingCfg.TxConfig.SignModeHandler().DefaultMode()

	err := txBuilder.SetMsgs(msg)
	if err != nil {
		return nil, err
	}

	txBuilder.SetGasLimit(100000)

	signerData := authsigning.SignerData{
		ChainID:       c.chainID,
		AccountNumber: c.account.AccountNumber,
		Sequence:      c.account.Sequence,
		PubKey:        c.publicKey,
		Address:       c.account.Address,
	}

	sigData := signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: nil,
	}
	sig := signing.SignatureV2{
		PubKey:   c.publicKey,
		Data:     &sigData,
		Sequence: c.account.Sequence,
	}

	if err := txBuilder.SetSignatures(sig); err != nil {
		return nil, err
	}

	sigV2, err := clienttx.SignWithPrivKey(
		signMode, signerData, txBuilder, &c.privateKey,
		encodingCfg.TxConfig, c.account.Sequence,
	)

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	txBytes, err := encodingCfg.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	return txBytes, nil
}
