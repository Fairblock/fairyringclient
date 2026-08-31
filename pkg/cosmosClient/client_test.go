package cosmosClient

import (
	"testing"

	"cosmossdk.io/math"
)

func TestCalculateFeeAmountRoundsUp(t *testing.T) {
	gasPrice, err := math.LegacyNewDecFromStr("0.025")
	if err != nil {
		t.Fatal(err)
	}

	if got := calculateFeeAmount(gasPrice, 300000).String(); got != "7500" {
		t.Fatalf("unexpected fee: got %s want 7500", got)
	}

	if got := calculateFeeAmount(gasPrice, 1).String(); got != "1" {
		t.Fatalf("fee must round up: got %s want 1", got)
	}
}
