package main

import (
	distIBE "DistributedIBE"
	"fairyringclient/cosmosClient"
	"fairyringclient/shareAPIClient"
	"math"
	"sync"
)

type KeyShare struct {
	Share distIBE.Share
	Index uint64
}

type ValidatorClients struct {
	mutex                   sync.Mutex
	CosmosClient            *cosmosClient.CosmosClient
	ShareApiClient          *shareAPIClient.ShareAPIClient
	CurrentShare            *KeyShare
	PendingShare            *KeyShare
	CurrentShareExpiryBlock uint64
}

func (v ValidatorClients) SetCurrentShare(share *KeyShare) {
	v.mutex.Lock()
	v.PendingShare = share
	v.mutex.Unlock()
}

func (v ValidatorClients) SetPendingShare(pendingShare *KeyShare) {
	v.mutex.Lock()
	v.PendingShare = pendingShare
	v.mutex.Unlock()
}

func (v ValidatorClients) SetExpiryBlock(blockNum uint64) {
	v.mutex.Lock()
	v.CurrentShareExpiryBlock = blockNum
	v.mutex.Unlock()
}

func (v ValidatorClients) ActivatePendingShare() {
	v.mutex.Lock()
	v.CurrentShare = v.PendingShare
	v.PendingShare = nil
	v.mutex.Unlock()
}

func (v ValidatorClients) SetupShareClient(
	endpoint string,
	chainID string,
	pks []string,
	totalValidatorNum uint64,
) (string, error) {
	threshold := uint64(math.Ceil(float64(totalValidatorNum) * (2.0 / 3.0)))

	result, err := v.ShareApiClient.Setup(chainID, endpoint, totalValidatorNum, threshold, pks)
	if err != nil {
		return "", err
	}

	return result.TxHash, nil
}
