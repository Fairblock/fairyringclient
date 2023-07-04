package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
)

func GenerateRSAKey(privateKeyPath, publicKeyPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	pemPrivateKeyFile, err := os.Create(privateKeyPath)
	if err != nil {
		return err
	}

	pemPrivateKey := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err = pem.Encode(pemPrivateKeyFile, &pemPrivateKey); err != nil {
		return err
	}

	pemPrivateKeyFile.Close()

	pemPublicKeyFile, err := os.Create(publicKeyPath)
	if err != nil {
		return err
	}

	pemPublicKey := pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}

	if err = pem.Encode(pemPublicKeyFile, &pemPublicKey); err != nil {
		return err
	}

	pemPublicKeyFile.Close()

	return nil
}
