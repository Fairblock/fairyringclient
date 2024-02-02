package shareAPIClient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	distIBE "github.com/FairBlock/DistributedIBE"
	"github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/types/time"
	dcrdSecp256k1 "github.com/decred/dcrd/dcrec/secp256k1"
	bls "github.com/drand/kyber-bls12381"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

type ShareAPIClient struct {
	URL          string
	Client       http.Client
	dcrdPubKey   dcrdSecp256k1.PublicKey
	base64PubKey string
	dcrdPrivKey  dcrdSecp256k1.PrivateKey
}

func NewShareAPIClient(url, privateKeHex string) (*ShareAPIClient, error) {
	keyBytes, err := hex.DecodeString(privateKeHex)
	if err != nil {
		return nil, err
	}

	dcrdPrivKey, dcrdPubKey := dcrdSecp256k1.PrivKeyFromBytes(keyBytes)

	pubKeyBytes := dcrdPubKey.Serialize()
	base64PubKey := make([]byte, base64.StdEncoding.EncodedLen(len(pubKeyBytes)))

	base64.StdEncoding.Encode(base64PubKey, pubKeyBytes)

	return &ShareAPIClient{
		URL:          url,
		Client:       http.Client{},
		dcrdPubKey:   *dcrdPubKey,
		dcrdPrivKey:  *dcrdPrivKey,
		base64PubKey: string(base64PubKey),
	}, nil
}

func (s ShareAPIClient) signMessage(message []byte) (string, error) {
	msgHash := sha256.New()
	_, err := msgHash.Write(message)
	if err != nil {
		return "", err
	}
	msgHashSum := msgHash.Sum(nil)

	sig, err := dcrdSecp256k1.SignCompact(&s.dcrdPrivKey, msgHashSum, false)
	if err != nil {
		log.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

func (s ShareAPIClient) decryptShare(shareCipher string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(shareCipher)
	if err != nil {
		return nil, err
	}

	plainByte, err := dcrdSecp256k1.Decrypt(&s.dcrdPrivKey, decoded)
	if err != nil {
		return nil, err
	}

	return plainByte, nil
}

func (s ShareAPIClient) doRequest(path, method, body string) ([]byte, error) {
	req, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	req.Header.Add("Content-Type", "application/json")
	if err != nil {
		return nil, err
	}

	res, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return resBody, nil
}

func (s ShareAPIClient) GetShare(msg string) (*distIBE.Share, uint64, error) {
	signed, err := s.signMessage([]byte(msg))
	if err != nil {
		return nil, 0, err
	}

	body := GetShareReq{
		Path:       "/share-req",
		HttpMethod: "GET",
		QueryStringParameters: GetShareParam{
			PublicKey: s.base64PubKey,
			Msg:       msg,
			SignedMsg: signed,
		},
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.doRequest("/share-req", "GET", string(jsonStr))
	if err != nil {
		return nil, 0, err
	}

	var parsedResp Response
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return nil, 0, err
	}

	var parsedGetShareResp GetShareRespBody
	err = json.Unmarshal([]byte(parsedResp.Body), &parsedGetShareResp)

	if len(parsedGetShareResp.EncShare) == 0 && len(parsedGetShareResp.Pk) == 0 {
		return nil, 0, errors.New(fmt.Sprintf("Get Share Resp is empty, body: %s", parsedResp.Body))
	}

	log.Printf("Got Encrypted Share: %s\n", parsedGetShareResp.EncShare)

	decryptedShare, err := s.decryptShare(parsedGetShareResp.EncShare)
	if err != nil {
		return nil, 0, err
	}

	parsedShare := bls.NewKyberScalar()
	err = parsedShare.UnmarshalBinary(decryptedShare)
	if err != nil {
		return nil, 0, err
	}

	indexByte, err := hex.DecodeString(parsedGetShareResp.Index)
	if err != nil {
		return nil, 0, err
	}

	indexInt := big.NewInt(0).SetBytes(indexByte).Uint64()

	return &distIBE.Share{
		Index: bls.NewKyberScalar().SetInt64(int64(indexInt)),
		Value: parsedShare,
	}, indexInt, nil
}

func (s ShareAPIClient) GetLastShare(msg string) (*distIBE.Share, uint64, error) {
	signed, err := s.signMessage([]byte(msg))
	if err != nil {
		return nil, 0, err
	}

	body := GetShareReq{
		Path:       "/last-share-req",
		HttpMethod: "GET",
		QueryStringParameters: GetShareParam{
			PublicKey: s.base64PubKey,
			Msg:       msg,
			SignedMsg: signed,
		},
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.doRequest("/share-req", "GET", string(jsonStr))
	if err != nil {
		return nil, 0, err
	}

	var parsedResp Response
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return nil, 0, err
	}

	var parsedGetShareResp GetShareRespBody
	err = json.Unmarshal([]byte(parsedResp.Body), &parsedGetShareResp)

	if len(parsedGetShareResp.EncShare) == 0 && len(parsedGetShareResp.Pk) == 0 {
		return nil, 0, errors.New(fmt.Sprintf("Get Last Share Resp is empty, body: %s", parsedResp.Body))
	}

	log.Printf("Got Last Encrypted Share: %s\n", parsedGetShareResp.EncShare)

	decryptedShare, err := s.decryptShare(parsedGetShareResp.EncShare)
	if err != nil {
		return nil, 0, err
	}

	parsedShare := bls.NewKyberScalar()
	err = parsedShare.UnmarshalBinary(decryptedShare)
	if err != nil {
		return nil, 0, err
	}

	indexByte, err := hex.DecodeString(parsedGetShareResp.Index)
	if err != nil {
		return nil, 0, err
	}

	indexInt := big.NewInt(0).SetBytes(indexByte).Uint64()

	return &distIBE.Share{
		Index: bls.NewKyberScalar().SetInt64(int64(indexInt)),
		Value: parsedShare,
	}, indexInt, nil
}

func (s ShareAPIClient) GetMasterPublicKey() (string, error) {
	body := GetMasterPublicKeyReq{
		Path:       "/mpk-req",
		HttpMethod: "GET",
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	res, err := s.doRequest("/mpk-req", "GET", string(jsonStr))
	if err != nil {
		return "", err
	}

	var parsedResp Response
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return "", err
	}

	if parsedResp.Body == "" {
		return "", errors.New("error getting master public key")
	}

	var pubVals PublicVals
	err = json.Unmarshal([]byte(parsedResp.Body), &pubVals)
	if err != nil {
		return "", err
	}

	return pubVals.MPK, nil
}

func (s ShareAPIClient) Setup(
	chainID, endpoint string,
	n, t uint64,
	pkList []string,
) (*SetupResult, error) {
	msg := strconv.FormatInt(time.Now().Unix(), 10)
	signed, err := s.signMessage([]byte(msg))
	if err != nil {
		return nil, err
	}

	body := SetupReq{
		Path:       "/setup",
		HttpMethod: "POST",
		QueryStringParameters: SetupParam{
			N:         strconv.FormatUint(n, 10),
			T:         strconv.FormatUint(t, 10),
			Msg:       msg,
			ChainID:   chainID,
			Endpoint:  endpoint,
			SignedMsg: signed,
		},
		MultiValueQueryStringParameters: SetupMultiValParam{
			PkList: pkList,
		},
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res, err := s.doRequest("/setup", "POST", string(jsonStr))
	if err != nil {
		return nil, err
	}

	var parsedResp Response
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return nil, err
	}

	if parsedResp.Body == "" {
		return nil, errors.New(fmt.Sprintf("error setting up, setup response: %s", string(res)))
	}

	var setupResult SetupResult
	err = json.Unmarshal([]byte(parsedResp.Body), &setupResult)
	if err != nil {
		return nil, err
	}

	if len(setupResult.Error) > 0 {
		return nil, errors.New(setupResult.Error)
	}

	return &setupResult, nil
}
