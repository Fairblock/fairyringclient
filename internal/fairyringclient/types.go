package fairyringclient

import (
	"encoding/hex"
	"fairyringclient/pkg/cosmosClient"
	"log"
	"strings"

	distIBE "github.com/FairBlock/DistributedIBE"
	"github.com/Fairblock/fairyring/x/keyshare/types"
	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"
	"github.com/pkg/errors"
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
	v.InvalidShareInARow++
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

func (v *ValidatorClients) RemovePendingShare() {
	v.PendingShare = nil
	v.PendingShareExpiryBlock = 0
}

func remainingBlocks(expiry uint64, height uint64) uint64 {
	if expiry <= height {
		return 0
	}
	return expiry - height
}

func (v *ValidatorClients) logShareState(height uint64) {
	if v.CurrentShare != nil {
		log.Printf(
			"Current Share Index: %d | Expires at: %d, in %d blocks",
			v.CurrentShare.Index,
			v.CurrentShareExpiryBlock,
			remainingBlocks(v.CurrentShareExpiryBlock, height),
		)
	}

	if v.PendingShare != nil {
		log.Printf(
			"Pending Share Index: %d | Expires at: %d, in %d blocks",
			v.PendingShare.Index,
			v.PendingShareExpiryBlock,
			remainingBlocks(v.PendingShareExpiryBlock, height),
		)
	}
}

func (v *ValidatorClients) resetAfterShareSwitch() {
	v.ResetInvalidShareNum()

	if v.Paused {
		v.Unpause()
		log.Printf("Client Unpaused, Current invalid share count: %d\n", v.InvalidShareInARow)
	}
}

func (v *ValidatorClients) SyncCurrentShareWithChain(latestBlockHeight uint64) error {
	if v.CurrentShare == nil {
		log.Println("Current Share not found, Getting Share from FairyRing")
		if err := v.UpdateKeyShareFromChain(false); err != nil {
			return err
		}
	}

	// If the local current share has already expired on-chain, do not fetch the
	// pending share. At this point the old pending share is already the chain's
	// active share, so either activate the locally cached pending share or fetch
	// the active share again from the chain.
	if v.CurrentShareExpiryBlock != 0 && v.CurrentShareExpiryBlock <= latestBlockHeight {
		log.Printf(
			"Local current share expired at block %d, latest block is %d. Syncing active share from chain\n",
			v.CurrentShareExpiryBlock,
			latestBlockHeight,
		)

		v.RemoveCurrentShare()

		if v.PendingShare != nil && v.PendingShareExpiryBlock > latestBlockHeight {
			v.ActivatePendingShare()
			log.Printf("Activated locally cached pending key share | Index: %d\n", v.CurrentShare.Index)
		} else {
			v.RemovePendingShare()
			if err := v.UpdateKeyShareFromChain(false); err != nil {
				return err
			}
			log.Printf("Fetched active key share from chain | Index: %d\n", v.CurrentShare.Index)
		}

		v.resetAfterShareSwitch()
	}

	return nil
}

func (v *ValidatorClients) PrepareShareForTargetHeight(targetHeight uint64) error {
	if v.CurrentShare == nil {
		log.Println("Current Share not found, Getting Share from FairyRing")
		if err := v.UpdateKeyShareFromChain(false); err != nil {
			return err
		}
	}

	// Blockwise keyshares are submitted for the next height. If the current share
	// expires at or before that target height, the keyshare for that height must
	// be derived from the queued share.
	if v.CurrentShareExpiryBlock != 0 && v.CurrentShareExpiryBlock <= targetHeight {
		log.Println("Current share expires before target height, switching to the queued one")
		v.RemoveCurrentShare()

		if v.PendingShare == nil || v.PendingShareExpiryBlock <= targetHeight {
			v.RemovePendingShare()
			log.Println("Pending share not found or stale, Getting pending share from FairyRing now")
			if err := v.UpdateKeyShareFromChain(true); err != nil {
				return err
			}
		}

		v.ActivatePendingShare()
		v.resetAfterShareSwitch()
		log.Printf("Activated pending key share | Index: %d\n", v.CurrentShare.Index)
	}

	return nil
}

func (v *ValidatorClients) UpdateKeyShareFromChain(forNextRound bool) error {
	share, shareIndex, expiry, err := v.CosmosClient.GetKeyShare(forNextRound)
	if err != nil {
		return err
	}

	commits, err := v.CosmosClient.GetCommitments()
	if err != nil {
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
	if err = newCommitmentPoint.UnmarshalBinary(newByteCommitment); err != nil {
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

	qid := hG2.Hash([]byte("verifying"))

	return distIBE.VerifyShare(s, newCommitment, extracted, qid), nil
}
