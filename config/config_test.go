package config

import "testing"

func TestDefaultConfigIncludesMainnetFeeSettings(t *testing.T) {
	cfg := DefaultConfig(false)

	if cfg.FairyRingNode.Denom != DefaultDenom {
		t.Fatalf("unexpected default denom: got %q want %q", cfg.FairyRingNode.Denom, DefaultDenom)
	}
	if cfg.FairyRingNode.GasPrice != DefaultGasPrice {
		t.Fatalf("unexpected default gas price: got %q want %q", cfg.FairyRingNode.GasPrice, DefaultGasPrice)
	}
	if cfg.SubmitBlockwiseKeyshares {
		t.Fatal("blockwise keyshare submission must default to disabled")
	}
}
