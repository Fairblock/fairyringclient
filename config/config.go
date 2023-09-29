package config

import (
	"fairyringclient/pkg/rsa"
	"fmt"
	"github.com/cometbft/cometbft/crypto"
	"github.com/spf13/viper"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

const (
	DefaultFolderName    = ".fairyringclient"
	DefaultKeyFolderName = DefaultFolderName + "/keys"
	DefaultChainID       = "fairytest-1"
	DefaultDenom         = "ufairy"
	DefaultShareAPIUrl   = "https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com"
)

type Node struct {
	Protocol string
	IP       string
	Port     uint64
	GRPCPort uint64
	Denom    string
	ChainID  string
}

type Config struct {
	FairyRingNode     Node
	PrivateKeys       []string
	ShareAPIUrl       string
	IsManager         bool
	TotalValidatorNum uint64
	MasterPrivateKey  string
}

func ReadConfigFromFile() (*Config, error) {
	var cfg Config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	viper.SetConfigName("config")
	viper.AddConfigPath(homeDir + "/" + DefaultFolderName)
	viper.SetConfigType("yml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	err = viper.Unmarshal(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func GetDefaultKeysDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir + "/" + DefaultKeyFolderName, nil
}

func (c *Config) GetFairyRingNodeURI() string {
	nodeURI := c.FairyRingNode.Protocol + "://" + c.FairyRingNode.IP + ":" + strconv.FormatUint(c.FairyRingNode.Port, 10)
	return nodeURI
}

func (c *Config) GetGRPCEndpoint() string {
	ep := c.FairyRingNode.IP + ":" + strconv.FormatUint(c.FairyRingNode.GRPCPort, 10)
	return ep
}

func (c *Config) SaveConfig() error {
	updateConfig(*c)

	if err := viper.WriteConfig(); err != nil {
		fmt.Errorf("failed to write config as : %s", err.Error())
	}

	return nil
}

func (c *Config) ExportConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stat(homeDir + "/" + DefaultFolderName); os.IsNotExist(err) {
		err = os.MkdirAll(homeDir+"/"+DefaultFolderName, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	if _, err := os.Stat(homeDir + "/" + DefaultKeyFolderName); os.IsNotExist(err) {
		err = os.MkdirAll(homeDir+"/"+DefaultKeyFolderName, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	filePath := filepath.Join(homeDir+"/"+DefaultFolderName, "config.yml")
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		// File does not exist, create it
		log.Println("Initializing FairyRing Client default config...")

		file, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file: %v", err)
		}
		defer file.Close()
		log.Printf("Config created at: %s\n", filePath)
	} else {
		log.Printf("Config file already exists: %s\n", filePath)
	}

	viper.SetConfigName("config")
	viper.AddConfigPath(homeDir + "/" + DefaultFolderName)
	viper.SetConfigType("yml")

	setInitialConfig(*c)

	if err = viper.WriteConfigAs(homeDir + "/" + DefaultFolderName + "/config.yml"); err != nil {
		fmt.Errorf("failed to write config as : %s", err.Error())
	}

	return nil
}

func GenerateRSAKey() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	pubKeyPath := homeDir + "/" + DefaultKeyFolderName + "/pk1.pem"
	secretKeyPath := homeDir + "/" + DefaultKeyFolderName + "/sk1.pem"

	if data, err := os.Stat(pubKeyPath); !os.IsNotExist(err) {
		// Only Not Exists error is expected
		return err
	} else if data != nil {
		fmt.Printf("RSA Pub key found in: %s, RSA Key will not be created...\n", pubKeyPath)
		return nil
	}

	if data, err := os.Stat(secretKeyPath); !os.IsNotExist(err) {
		return err
	} else if data != nil {
		fmt.Printf("RSA Secret key found in: %s, RSA Key will not be created...\n", secretKeyPath)
		return nil
	}

	if err := rsa.GenerateRSAKey(secretKeyPath, pubKeyPath); err != nil {
		return err
	}
	return nil
}

func DefaultConfig(withCosmosKey bool) Config {
	var privateKeys []string

	if withCosmosKey {
		privateKeys = append(privateKeys, crypto.CRandHex(64))
	}

	return Config{
		FairyRingNode: Node{
			Protocol: "http",
			IP:       "127.0.0.1",
			Port:     26657,
			GRPCPort: 9090,
			Denom:    DefaultDenom,
			ChainID:  DefaultChainID,
		},
		PrivateKeys:       privateKeys,
		ShareAPIUrl:       DefaultShareAPIUrl,
		IsManager:         false,
		TotalValidatorNum: 0,
		MasterPrivateKey:  "",
	}
}

func updateConfig(c Config) {
	viper.Set("FairyRingNode.ip", c.FairyRingNode.IP)
	viper.Set("FairyRingNode.port", c.FairyRingNode.Port)
	viper.Set("FairyRingNode.protocol", c.FairyRingNode.Protocol)
	viper.Set("FairyRingNode.grpcPort", c.FairyRingNode.GRPCPort)
	viper.Set("FairyRingNode.denom", c.FairyRingNode.Denom)
	viper.Set("FairyRingNode.chainID", c.FairyRingNode.ChainID)

	viper.Set("ShareAPIUrl", c.ShareAPIUrl)
	viper.Set("PrivateKeys", c.PrivateKeys)
	viper.Set("IsManager", c.IsManager)
}

func setInitialConfig(c Config) {
	viper.SetDefault("FairyRingNode.ip", c.FairyRingNode.IP)
	viper.SetDefault("FairyRingNode.port", c.FairyRingNode.Port)
	viper.SetDefault("FairyRingNode.protocol", c.FairyRingNode.Protocol)
	viper.SetDefault("FairyRingNode.grpcPort", c.FairyRingNode.GRPCPort)
	viper.SetDefault("FairyRingNode.denom", c.FairyRingNode.Denom)
	viper.SetDefault("FairyRingNode.chainID", c.FairyRingNode.ChainID)

	viper.SetDefault("ShareAPIUrl", c.ShareAPIUrl)
	viper.SetDefault("PrivateKeys", c.PrivateKeys)
	viper.SetDefault("IsManager", c.IsManager)
}
