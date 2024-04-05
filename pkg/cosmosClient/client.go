package cosmosClient

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	distIBE "github.com/FairBlock/DistributedIBE"
	dcrdSecp256k1 "github.com/decred/dcrd/dcrec/secp256k1"
	bls "github.com/drand/kyber-bls12381"
	"log"
	"strings"
	"time"

	"cosmossdk.io/math"
	"github.com/Fairblock/fairyring/app"
	keysharetypes "github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/Fairblock/fairyring/x/pep/types"
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
	defaultGasAdjustment = 1.5
	defaultGasLimit      = 300000
)

type CosmosClient struct {
	authClient          authtypes.QueryClient
	txClient            tx.ServiceClient
	grpcConn            *grpc.ClientConn
	bankQueryClient     banktypes.QueryClient
	pepQueryClient      types.QueryClient
	keyshareQueryClient keysharetypes.QueryClient
	privateKey          secp256k1.PrivKey
	dcrdPrivKey         dcrdSecp256k1.PrivateKey
	publicKey           cryptotypes.PubKey
	account             authtypes.BaseAccount
	accAddress          cosmostypes.AccAddress
	chainID             string
}

func PrivateKeyToAccAddress(privateKeyHex string) (cosmostypes.AccAddress, error) {
	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, err
	}

	privateKey := secp256k1.PrivKey{Key: keyBytes}

	return cosmostypes.AccAddress(privateKey.PubKey().Address()), nil
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
	pepeClient := types.NewQueryClient(grpcConn)
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
	}, nil
}

func (c *CosmosClient) IsAddrAuthorized(target string) bool {
	resp, err := c.keyshareQueryClient.AuthorizedAddress(
		context.Background(),
		&keysharetypes.QueryGetAuthorizedAddressRequest{
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

func (c *CosmosClient) GetActivePubKey() (*keysharetypes.QueryPubKeyResponse, error) {
	resp, err := c.keyshareQueryClient.PubKey(
		context.Background(),
		&keysharetypes.QueryPubKeyRequest{},
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

	targetEncKeyShareList := pubKey.ActivePubKey.EncryptedKeyShares

	if getPendingShare {
		targetEncKeyShareList = pubKey.QueuedPubKey.EncryptedKeyShares
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

			expiryHeight := pubKey.ActivePubKey.Expiry

			if getPendingShare {
				expiryHeight = pubKey.QueuedPubKey.Expiry
			}

			return parsedShare, uint64(keyShareIndex), expiryHeight, nil
		}
	}
	return nil, 0, 0, errors.New("encrypted share for your validator not found")
}

func (c *CosmosClient) GetLatestHeight() (uint64, error) {
	resp, err := c.pepQueryClient.LatestHeight(
		context.Background(),
		&types.QueryLatestHeightRequest{},
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

func (c *CosmosClient) BroadcastTx(msg cosmostypes.Msg, adjustGas bool) (*cosmostypes.TxResponse, error) {
	txBytes, err := c.signTxMsg(msg, adjustGas)
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
	encodingCfg := app.MakeEncodingConfig()
	txBuilder := encodingCfg.TxConfig.NewTxBuilder()
	signMode := encodingCfg.TxConfig.SignModeHandler().DefaultMode()

	err := txBuilder.SetMsgs(msg)
	if err != nil {
		return nil, err
	}

	var newGasLimit uint64 = defaultGasLimit
	if adjustGas {
		txf := clienttx.Factory{}.
			WithGas(defaultGasLimit).
			WithSignMode(signMode).
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
