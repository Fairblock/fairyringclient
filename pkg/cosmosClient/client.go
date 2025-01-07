package cosmosClient

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	distIBE "github.com/FairBlock/DistributedIBE"
	keysharetypes "github.com/Fairblock/fairyring/x/keyshare/types"
	peptypes "github.com/Fairblock/fairyring/x/pep/types"
	dcrdSecp256k1 "github.com/decred/dcrd/dcrec/secp256k1"
	bls "github.com/drand/kyber-bls12381"
	"github.com/skip-mev/block-sdk/v2/testutils"
	"log"
	"strings"
	"time"

	"cosmossdk.io/math"

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
)

const (
	defaultGasAdjustment = 3
	defaultGasLimit      = 300000
)

type QueuedTx struct {
	Tx                 *cosmostypes.Msg
	TxResultErrHandler func(error)
	TxSuccessHandler   func(*tx.GetTxResponse)
	AdjustGas          bool
}

type CosmosClient struct {
	authClient          authtypes.QueryClient
	txClient            tx.ServiceClient
	grpcConn            *grpc.ClientConn
	bankQueryClient     banktypes.QueryClient
	pepQueryClient      peptypes.QueryClient
	keyshareQueryClient keysharetypes.QueryClient
	privateKey          secp256k1.PrivKey
	dcrdPrivKey         dcrdSecp256k1.PrivateKey
	publicKey           cryptotypes.PubKey
	account             authtypes.BaseAccount
	accAddress          cosmostypes.AccAddress
	chainID             string
	txQueue             chan QueuedTx
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
	pepeClient := peptypes.NewQueryClient(grpcConn)
	keyshareClient := keysharetypes.NewQueryClient(grpcConn)

	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, err
	}

	privateKey := secp256k1.PrivKey{Key: keyBytes}
	pubKey := privateKey.PubKey()
	address := pubKey.Address()

	dcrdPrivKey, _ := dcrdSecp256k1.PrivKeyFromBytes(keyBytes)

	cfg := cosmostypes.GetConfig()
	cfg.SetBech32PrefixForAccount("fairy", "fairypub")
	cfg.SetBech32PrefixForValidator("fairyvaloper", "fairyvaloperpub")
	cfg.SetBech32PrefixForConsensusNode("fairyvalcons", "fairyrvalconspub")

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
		bankQueryClient:     bankClient,
		authClient:          authClient,
		txClient:            tx.NewServiceClient(grpcConn),
		pepQueryClient:      pepeClient,
		keyshareQueryClient: keyshareClient,
		grpcConn:            grpcConn,
		privateKey:          privateKey,
		dcrdPrivKey:         *dcrdPrivKey,
		account:             baseAccount,
		accAddress:          accAddr,
		publicKey:           pubKey,
		chainID:             chainID,
		txQueue:             make(chan QueuedTx, 1),
	}, nil
}

func (c *CosmosClient) updateAccSequence() error {
	out, err := c.authClient.Account(context.Background(),
		&authtypes.QueryAccountRequest{Address: c.accAddress.String()})
	if err != nil {
		return err
	}
	var baseAccount authtypes.BaseAccount

	if err = baseAccount.Unmarshal(out.Account.Value); err != nil {
		return err
	}

	c.account = baseAccount
	return nil
}

func (c *CosmosClient) IsAddrAuthorized(target string) bool {
	resp, err := c.keyshareQueryClient.AuthorizedAddress(
		context.Background(),
		&keysharetypes.QueryAuthorizedAddressRequest{
			Target: target,
		},
	)
	if err != nil {
		return false
	}
	return resp.AuthorizedAddress.IsAuthorized
}

func (c *CosmosClient) GetCommitments() (*keysharetypes.QueryCommitmentsResponse, error) {
	resp, err := c.keyshareQueryClient.Commitments(
		context.Background(),
		&keysharetypes.QueryCommitmentsRequest{},
	)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *CosmosClient) GetActivePubKey() (*keysharetypes.QueryPubkeyResponse, error) {
	resp, err := c.keyshareQueryClient.Pubkey(
		context.Background(),
		&keysharetypes.QueryPubkeyRequest{},
	)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *CosmosClient) GetKeyShare(getPendingShare bool) (*distIBE.Share, uint64, uint64, error) {
	pubKey, err := c.GetActivePubKey()
	if err != nil {
		return nil, 0, 0, err
	}

	targetEncKeyShareList := pubKey.ActivePubkey.EncryptedKeyshares

	if getPendingShare {
		targetEncKeyShareList = pubKey.QueuedPubkey.EncryptedKeyshares
	}

	if len(targetEncKeyShareList) == 0 {
		return nil, 0, 0, errors.New("encrypted shares array for target round is empty")
	}

	for index, val := range targetEncKeyShareList {
		if val.Validator == c.GetAddress() {
			decryptedByte, err := c.decryptShare(val.Data)
			if err != nil {
				return nil, 0, 0, err
			}
			keyShareIndex := index + 1
			parsedShare, err := c.parseShare(decryptedByte, int64(keyShareIndex))
			if err != nil {
				return nil, 0, 0, err
			}

			expiryHeight := pubKey.ActivePubkey.Expiry

			if getPendingShare {
				expiryHeight = pubKey.QueuedPubkey.Expiry
			}

			return parsedShare, uint64(keyShareIndex), expiryHeight, nil
		}
	}
	return nil, 0, 0, errors.New("encrypted share for your validator not found")
}

func (c *CosmosClient) GetLatestHeight() (uint64, error) {
	resp, err := c.pepQueryClient.LatestHeight(
		context.Background(),
		&peptypes.QueryLatestHeightRequest{},
	)
	if err != nil {
		return 0, err
	}
	return resp.Height, nil
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

func (c *CosmosClient) AddTxToQueue(
	msg cosmostypes.Msg,
	adjustGas bool,
	errHandler func(error),
	successHandler func(*tx.GetTxResponse),
) {
	c.txQueue <- QueuedTx{
		Tx:                 &msg,
		AdjustGas:          adjustGas,
		TxResultErrHandler: errHandler,
		TxSuccessHandler:   successHandler,
	}
}

func (c *CosmosClient) HandleTxQueue() error {
	for {
		queuedTx := <-c.txQueue
		if queuedTx.Tx == nil {
			continue
		}

		go func(qTx QueuedTx) {
			txBytes, err := c.signTxMsg(*qTx.Tx, qTx.AdjustGas)
			if err != nil {
				qTx.TxResultErrHandler(errors.New(fmt.Sprintf("Error signing tx: %s", err.Error())))
				return
			}
			resp, err := c.txClient.BroadcastTx(
				context.Background(),
				&tx.BroadcastTxRequest{
					TxBytes: txBytes,
					Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
				},
			)
			if err != nil {
				log.Printf("Error broadcasting tx in Tx queue handler: %v", err)
				if qTx.TxResultErrHandler != nil {
					qTx.TxResultErrHandler(err)
				}
				return
			}
			if resp.TxResponse.Code != 0 {
				qTx.TxResultErrHandler(errors.New(fmt.Sprintf("Error broadcasting tx: %s", resp.TxResponse.RawLog)))
				return
			}
			c.WaitForQueuedTx(qTx, resp.TxResponse.TxHash)
		}(queuedTx)

	}
}

func (c *CosmosClient) WaitForQueuedTx(q QueuedTx, txHash string) {
	getTxResp, err := c.WaitForTx(txHash, time.Second)
	if err != nil {
		log.Printf("Error waiting for tx in Tx queue handler: %v", err)
		if q.TxResultErrHandler != nil {
			q.TxResultErrHandler(err)
		}
	} else if q.TxSuccessHandler != nil {
		q.TxSuccessHandler(getTxResp)
	}
}

func (c *CosmosClient) BroadcastTx(msg cosmostypes.Msg, adjustGas bool) (*tx.GetTxResponse, error) {
	if err := c.updateAccSequence(); err != nil {
		return nil, err
	}

	txBytes, err := c.signTxMsg(msg, adjustGas)
	if err != nil {
		return nil, err
	}

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

	for {
		getTxResp, err := c.txClient.GetTx(context.Background(), &tx.GetTxRequest{Hash: resp.TxResponse.TxHash})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				time.Sleep(time.Second)
				continue
			}
			return nil, err
		}
		return getTxResp, err
	}
}

func (c *CosmosClient) decryptShare(shareCipher string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(shareCipher)
	if err != nil {
		return nil, err
	}

	plainByte, err := dcrdSecp256k1.Decrypt(&c.dcrdPrivKey, decoded)
	if err != nil {
		return nil, err
	}

	return plainByte, nil
}

func (c *CosmosClient) parseShare(shareByte []byte, index int64) (*distIBE.Share, error) {
	parsedShare := bls.NewKyberScalar()
	err := parsedShare.UnmarshalBinary(shareByte)
	if err != nil {
		return nil, err
	}
	return &distIBE.Share{
		Index: bls.NewKyberScalar().SetInt64(index),
		Value: parsedShare,
	}, nil
}

func (c *CosmosClient) WaitForTx(hash string, rate time.Duration) (*tx.GetTxResponse, error) {
	for {
		resp, err := c.txClient.GetTx(context.Background(), &tx.GetTxRequest{Hash: hash})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				time.Sleep(rate)
				continue
			}
			return nil, err
		}
		return resp, err
	}
}

func (c *CosmosClient) signTxMsg(msg cosmostypes.Msg, adjustGas bool) ([]byte, error) {
	encodingCfg := testutils.CreateTestEncodingConfig()
	txBuilder := encodingCfg.TxConfig.NewTxBuilder()
	encodingCfg.TxConfig.SignModeHandler().DefaultMode()

	err := txBuilder.SetMsgs(msg)
	if err != nil {
		return nil, err
	}
	
	if err := c.updateAccSequence(); err != nil {
		log.Printf("Error updating Account sequence in Tx queue handler: %v", err)
		return nil, err
	}

	var newGasLimit uint64 = defaultGasLimit
	if adjustGas {
		txf := clienttx.Factory{}.
			WithGas(defaultGasLimit).
			WithSignMode(1).
			WithTxConfig(encodingCfg.TxConfig).
			WithChainID(c.chainID).
			WithAccountNumber(c.account.AccountNumber).
			WithSequence(c.account.Sequence).
			WithGasAdjustment(defaultGasAdjustment)

		_, newGasLimit, err = clienttx.CalculateGas(c.grpcConn, txf, msg)
		if err != nil {
			return nil, err
		}
	}

	txBuilder.SetGasLimit(newGasLimit)

	signerData := authsigning.SignerData{
		ChainID:       c.chainID,
		AccountNumber: c.account.AccountNumber,
		Sequence:      c.account.Sequence,
		PubKey:        c.publicKey,
		Address:       c.account.Address,
	}

	sigData := signing.SingleSignatureData{
		SignMode:  1,
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
		context.Background(), 1, signerData, txBuilder, &c.privateKey,
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
