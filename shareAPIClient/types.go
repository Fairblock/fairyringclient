package shareAPIClient

type QueryStringParameters interface{}
type MultiValueQueryStringParameters interface{}

type GetMasterPublicKeyReq struct {
	Path       string `json:"path"`
	HttpMethod string `json:"httpMethod"`
}

type GetShareReq struct {
	Path                  string        `json:"path"`
	HttpMethod            string        `json:"httpMethod"`
	QueryStringParameters GetShareParam `json:"queryStringParameters,omitempty"`
}

type SetupReq struct {
	Path                            string             `json:"path"`
	HttpMethod                      string             `json:"httpMethod"`
	QueryStringParameters           SetupParam         `json:"queryStringParameters,omitempty"`
	MultiValueQueryStringParameters SetupMultiValParam `json:"multiValueQueryStringParameters,omitempty"`
}

type GetShareParam struct {
	PublicKey string `json:"publicKey"`
	Msg       string `json:"msg"`
	SignedMsg string `json:"signedMsg"`
}

type SetupMultiValParam struct {
	PkList []string `json:"pkList"`
}

type SetupParam struct {
	N         string `json:"n"`
	T         string `json:"t"`
	Msg       string `json:"msg"`
	SignedMsg string `json:"signedMsg"`
}

type GetShareRespBody struct {
	Pk       string `json:"pk"`
	EncShare string `json:"encShare"`
	Index    string `json:"index"`
}

type PublicVals struct {
	Commits []string `json:"Commits"`
	MPK     string   `json:"MPK"`
}

type Response struct {
	Body string `json:"body"`
}
