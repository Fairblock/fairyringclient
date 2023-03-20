package shareAPIClient

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

func pemToPrivateKey(fileName string) (*rsa.PrivateKey, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	//Create a byte slice (pemBytes) the size of the file size
	pemFileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	pemBytes := make([]byte, pemFileInfo.Size())
	file.Read(pemBytes)
	if err != nil {
		return nil, err
	}

	data, _ := pem.Decode(pemBytes)
	if data == nil {
		return nil, errors.New("error in decode pem bytes")
	}

	if data.Type != privateKeyType {
		return nil, errors.New(fmt.Sprintf("incorrect key type, expecting pem file type %s, got %s", privateKeyType, data.Type))
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

func bytesToPemStr(b []byte, types string) (string, error) {
	var pemKey = &pem.Block{
		Type:  types,
		Bytes: b,
	}

	var dst bytes.Buffer
	err := pem.Encode(&dst, pemKey)
	if err != nil {
		return "", err
	}

	return dst.String(), nil
}

func getNowStr() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
