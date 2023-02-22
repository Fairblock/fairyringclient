package shareAPIClient

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"github.com/tendermint/tendermint/libs/json"
	"io"
	"log"
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

func (s ShareAPIClient) GetShare(msg string) (*GetShareRespBody, error) {
	signed, err := s.signMessage([]byte(msg))
	if err != nil {
		return nil, err
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
		return nil, err
	}

	res, err := s.doRequest("/share-req", "GET", string(jsonStr))
	if err != nil {
		return nil, err
	}

	var parsedResp GetShareResp
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return nil, err
	}

	return &parsedResp.Body, nil
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

	return parsedResp.Body, nil
}

func (s ShareAPIClient) Setup(n, t uint64, msg string, pkList []string) (string, error) {
	signed, err := s.signMessage([]byte(msg))
	if err != nil {
		return "", err
	}

	body := SetupReq{
		Path:       "/setup",
		HttpMethod: "POST",
		QueryStringParameters: SetupParam{
			N:         strconv.FormatUint(n, 10),
			T:         strconv.FormatUint(t, 10),
			Msg:       msg,
			SignedMsg: signed,
		},
		MultiValueQueryStringParameters: SetupMultiValParam{
			PkList: pkList,
		},
	}

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	res, err := s.doRequest("/setup", "POST", string(jsonStr))
	if err != nil {
		return "", err
	}

	var parsedResp Response
	err = json.Unmarshal(res, &parsedResp)
	if err != nil {
		return "", err
	}

	return parsedResp.Body, nil
}
