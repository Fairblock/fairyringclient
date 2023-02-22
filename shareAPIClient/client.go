package shareAPIClient

import (
	"github.com/tendermint/tendermint/libs/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type ShareAPIClient struct {
	URL    string
	Client http.Client
}

func NewShareAPIClient(url string) ShareAPIClient {
	return ShareAPIClient{
		URL:    url,
		Client: http.Client{},
	}
}

func (s ShareAPIClient) doRequest(path, method, body string) ([]byte, error) {
	req, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
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

func (s ShareAPIClient) GetShare(pubKey, msg, signedMsg string) (*GetShareRespBody, error) {
	body := Req{
		Path:       "/share-req",
		HttpMethod: "GET",
		QueryStringParameters: QueryStringParameters(GetShareParam{
			PublicKey: pubKey,
			Msg:       msg,
			SignedMsg: signedMsg,
		}),
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
	err = json.Unmarshal(res, parsedResp)
	if err != nil {
		return nil, err
	}

	return &parsedResp.Body, nil
}

func (s ShareAPIClient) GetMasterPublicKey() (string, error) {
	body := Req{
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
	err = json.Unmarshal(res, parsedResp)
	if err != nil {
		return "", err
	}

	return parsedResp.Body, nil
}

func (s ShareAPIClient) Setup(n, t uint64, msg, signedMsg string, pkList []string) (string, error) {
	body := Req{
		Path:       "/setup",
		HttpMethod: "POST",
		QueryStringParameters: QueryStringParameters(SetupParam{
			N:         strconv.FormatUint(n, 10),
			T:         strconv.FormatUint(t, 10),
			Msg:       msg,
			SignedMsg: signedMsg,
		}),
		MultiValueQueryStringParameters: MultiValueQueryStringParameters(SetupMultiValParam{
			PkList: pkList,
		}),
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
	err = json.Unmarshal(res, parsedResp)
	if err != nil {
		return "", err
	}

	return parsedResp.Body, nil
}
