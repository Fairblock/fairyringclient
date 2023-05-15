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
	Mutex                   sync.Mutex
	CosmosClient            *cosmosClient.CosmosClient
	ShareApiClient          *shareAPIClient.ShareAPIClient
	CurrentShare            *KeyShare
	PendingShare            *KeyShare
	CurrentShareExpiryBlock uint64
	PendingShareExpiryBlock uint64
}

func (v *ValidatorClients) SetCurrentShare(share *KeyShare) {
	v.Mutex.Lock()
	v.PendingShare = share
	v.Mutex.Unlock()
}

func (v *ValidatorClients) SetPendingShare(pendingShare *KeyShare) {
	v.Mutex.Lock()
	v.PendingShare = pendingShare
	v.Mutex.Unlock()
}

func (v *ValidatorClients) SetCurrentShareExpiryBlock(blockNum uint64) {
	v.Mutex.Lock()
	v.CurrentShareExpiryBlock = blockNum
	v.Mutex.Unlock()
}

func (v *ValidatorClients) SetPendingShareExpiryBlock(blockNum uint64) {
	v.Mutex.Lock()
	v.PendingShareExpiryBlock = blockNum
	v.Mutex.Unlock()
}

func (v *ValidatorClients) ActivatePendingShare() {
	v.Mutex.Lock()
	v.CurrentShare = v.PendingShare
	v.CurrentShareExpiryBlock = v.PendingShareExpiryBlock
	v.PendingShare = nil
	v.PendingShareExpiryBlock = 0
	v.Mutex.Unlock()
}

func (v *ValidatorClients) SetupShareClient(
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
