package fairyringclient

import (
	"encoding/hex"
	"fairyringclient/pkg/cosmosClient"
	distIBE "github.com/FairBlock/DistributedIBE"
	"github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"
	"github.com/pkg/errors"
	"log"
	"strings"
)

type KeyShare struct {
	Share *distIBE.Share
	Index uint64
}

type ValidatorClients struct {
	CosmosClient            *cosmosClient.CosmosClient
	Commitments             *types.QueryCommitmentsResponse
	CurrentShare            *KeyShare
	PendingShare            *KeyShare
	CurrentShareExpiryBlock uint64
	PendingShareExpiryBlock uint64
	InvalidShareInARow      uint64
	Paused                  bool
}

func (v *ValidatorClients) IsAccountAuthorized() bool {
	return v.CosmosClient.IsAddrAuthorized(v.CosmosClient.GetAddress())
}

func (v *ValidatorClients) RegisterValidatorSet() {
	addr := v.CosmosClient.GetAddress()
	_, err := validatorCosmosClient.CosmosClient.BroadcastTx(&types.MsgRegisterValidator{
		Creator: addr,
	}, true)
	if err != nil {
		if !strings.Contains(err.Error(), "validator already registered") {
			log.Fatal(err)
		}
	}
	log.Printf("%s Registered as Validator", addr)
}

//
//func (v *ValidatorClients) UnregisterValidatorSet() {
//	addr := v.CosmosClient.GetAddress()
//	_, err := validatorCosmosClient.CosmosClient.BroadcastTx(&types.MsgUnregisterValidator{
//		Creator: addr,
//	}, true)
//	if err != nil {
//		if !strings.Contains(err.Error(), "validator already unregistered") {
//			log.Fatal(err)
//		}
//	}
//	log.Printf("%s Unregistered Validator", addr)
//}

func (v *ValidatorClients) Pause() {
	v.Paused = true
}

func (v *ValidatorClients) Unpause() {
	v.Paused = false
}

func (v *ValidatorClients) SetCommitments(c *types.QueryCommitmentsResponse) {
	v.Commitments = c
}

func (v *ValidatorClients) IncreaseInvalidShareNum() {
	v.InvalidShareInARow = v.InvalidShareInARow + 1
}

func (v *ValidatorClients) ResetInvalidShareNum() {
	v.InvalidShareInARow = 0
}

func (v *ValidatorClients) ActivatePendingShare() {
	v.CurrentShare = v.PendingShare
	v.CurrentShareExpiryBlock = v.PendingShareExpiryBlock
	v.PendingShare = nil
	v.PendingShareExpiryBlock = 0
}

func (v *ValidatorClients) RemoveCurrentShare() {
	v.CurrentShare = nil
	v.CurrentShareExpiryBlock = 0
}

func (v *ValidatorClients) UpdateKeyShareFromChain(forNextRound bool) error {
	share, shareIndex, expiry, err := v.CosmosClient.GetKeyShare(forNextRound)
	if err != nil {
		return err
	}

	commits, err := v.CosmosClient.GetCommitments()
	for err != nil {
		return err
	}

	keyShare := &KeyShare{
		Share: share,
		Index: shareIndex,
	}

	if forNextRound {
		v.PendingShare = keyShare
		v.PendingShareExpiryBlock = expiry
	} else {
		v.CurrentShare = keyShare
		v.CurrentShareExpiryBlock = expiry
	}

	targetCommits := commits.ActiveCommitments
	if forNextRound {
		targetCommits = commits.QueuedCommitments
	}

	valid, err := v.VerifyShare(targetCommits, forNextRound)
	if err != nil {
		return err
	}

	if !valid {
		return errors.New("got invalid share on chain")
	}

	v.Commitments = commits

	return nil
}

func (v *ValidatorClients) DeriveKeyShare(id []byte) (string, uint64, error) {
	s := bls.NewBLS12381Suite()
	extractedKey := distIBE.Extract(s, v.CurrentShare.Share.Value, uint32(v.CurrentShare.Index), id)
	extractedKeyBinary, err := extractedKey.SK.MarshalBinary()
	if err != nil {
		return "", 0, err
	}
	extractedKeyHex := hex.EncodeToString(extractedKeyBinary)
	return extractedKeyHex, v.CurrentShare.Index, nil
}

func (v *ValidatorClients) VerifyShare(commitments *types.Commitments, verifyPendingShare bool) (bool, error) {
	s := bls.NewBLS12381Suite()

	if len(commitments.Commitments) == 0 {
		return false, errors.New("Commitment provided is empty")
	}

	targetShare := v.CurrentShare

	if targetShare == nil {
		return false, errors.New("active share not found")
	}

	if verifyPendingShare {
		if v.PendingShare == nil {
			return false, errors.New("verify pending share but pending share not found")
		}
		targetShare = v.PendingShare
	}

	targetCommitments := commitments.Commitments

	extracted := distIBE.Extract(s, targetShare.Share.Value, uint32(targetShare.Index), []byte("verifying"))

	newByteCommitment, err := hex.DecodeString(targetCommitments[targetShare.Index-1])
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
