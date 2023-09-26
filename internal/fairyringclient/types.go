package fairyringclient

import (
	"fairyringclient/pkg/cosmosClient"
	"fairyringclient/pkg/shareAPIClient"
	distIBE "github.com/FairBlock/DistributedIBE"
	"math"
)

type KeyShare struct {
	Share distIBE.Share
	Index uint64
}

type ValidatorClients struct {
	CosmosClient            *cosmosClient.CosmosClient
	ShareApiClient          *shareAPIClient.ShareAPIClient
	CurrentShare            *KeyShare
	PendingShare            *KeyShare
	CurrentShareExpiryBlock uint64
	PendingShareExpiryBlock uint64
}

func (v *ValidatorClients) SetCurrentShare(share *KeyShare) {
	v.CurrentShare = share
}

func (v *ValidatorClients) SetPendingShare(pendingShare *KeyShare) {
	v.PendingShare = pendingShare
}

func (v *ValidatorClients) SetCurrentShareExpiryBlock(blockNum uint64) {
	v.CurrentShareExpiryBlock = blockNum
}

func (v *ValidatorClients) SetPendingShareExpiryBlock(blockNum uint64) {
	v.PendingShareExpiryBlock = blockNum
}

func (v *ValidatorClients) ActivatePendingShare() {
	v.CurrentShare = v.PendingShare
	v.CurrentShareExpiryBlock = v.PendingShareExpiryBlock
	v.PendingShare = nil
	v.PendingShareExpiryBlock = 0
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
