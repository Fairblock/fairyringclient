package fairyringclient

import (
	"encoding/hex"
	"errors"
	"fairyring/x/keyshare/types"
	"fairyringclient/pkg/cosmosClient"
	"fairyringclient/pkg/shareAPIClient"
	distIBE "github.com/FairBlock/DistributedIBE"
	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"
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
	InvalidShareInARow      uint64
	Paused                  bool
}

func (v *ValidatorClients) Pause() {
	v.Paused = true
}

func (v *ValidatorClients) Unpause() {
	v.Paused = false
}

func (v *ValidatorClients) IncreaseInvalidShareNum() {
	v.InvalidShareInARow = v.InvalidShareInARow + 1
}

func (v *ValidatorClients) ResetInvalidShareNum() {
	v.InvalidShareInARow = 0
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

func (v *ValidatorClients) VerifyShare(commitments *types.QueryCommitmentsResponse, verifyPendingShare bool) (bool, error) {
	s := bls.NewBLS12381Suite()

	targetShare := v.CurrentShare
	if verifyPendingShare {
		if v.PendingShare == nil {
			return false, errors.New("verify pending share but pending share not found")
		}
		targetShare = v.PendingShare
	}

	targetCommitments := commitments.ActiveCommitments.Commitments
	if verifyPendingShare {
		if commitments.QueuedCommitments == nil {
			return false, errors.New("verify pending share but queued commitment not found")
		}
		targetCommitments = commitments.QueuedCommitments.Commitments
	}

	extracted := distIBE.Extract(s, targetShare.Share.Value, uint32(targetShare.Index), []byte("verifying"))

	newByteCommitment, err := hex.DecodeString(targetCommitments[targetShare.Index])
	if err != nil {
		return false, err
	}

	newCommitmentPoint := s.G1().Point()
	err = newCommitmentPoint.UnmarshalBinary(newByteCommitment)
	if err != nil {
		return false, err
	}

	newCommitment := distIBE.Commitment{
		SP:    newCommitmentPoint,
		Index: uint32(targetShare.Index),
	}

	hG2, ok := s.G2().Point().(kyber.HashablePoint)
	if !ok {
		return false, errors.New("unable to create hashable G2 point")
	}

	Qid := hG2.Hash([]byte("verifying"))

	return distIBE.VerifyShare(s, newCommitment, extracted, Qid), nil
}
