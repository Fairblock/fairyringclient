package shareAPIClient

import (
	distIBE "DistributedIBE"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	bls "github.com/drand/kyber-bls12381"
	"github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/types/time"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

type ShareAPIClient struct {
	URL        string
	Client     http.Client
	pubKey     string
	privateKey rsa.PrivateKey
}

func NewShareAPIClient(url, privateKeyPath string) (*ShareAPIClient, error) {
	pKey, err := pemToPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	pubKeyStr, err := bytesToPemStr(x509.MarshalPKCS1PublicKey(&pKey.PublicKey), publicKeyType)
	if err != nil {
		return nil, err
	}

	return &ShareAPIClient{
		URL:        url,
		Client:     http.Client{},
		pubKey:     pubKeyStr,
		privateKey: *pKey,
	}, nil
}

func (s ShareAPIClient) signMessage(message []byte) (string, error) {
	msgHash := sha256.New()
	_, err := msgHash.Write(message)
	if err != nil {
		return "", err
	}
	msgHashSum := msgHash.Sum(nil)

	sig, err := rsa.SignPSS(rand.Reader, &s.privateKey, crypto.SHA256, msgHashSum, nil)
	if err != nil {
		log.Fatal(err)
	}

	// return base64.StdEncoding.EncodeToString(sig), nil
	signature, err := bytesToPemStr(sig, signatureType)

	return signature, nil
}

func (s ShareAPIClient) decryptShare(shareCipher string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(shareCipher)
	if err != nil {
		return nil, err
	}

	label := []byte("OAEP Encrypted")
	plainByte, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, &s.privateKey, decoded, label)
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
			PublicKey: s.pubKey,
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
			PublicKey: s.pubKey,
			Msg:       msg,
			SignedMsg: signed,
		},
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.doRequest("/last-share-req", "GET", string(jsonStr))
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
