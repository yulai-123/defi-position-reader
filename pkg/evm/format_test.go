package evm

import (
	"math/big"
	"testing"
)

func TestFormatUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *big.Int
		decimals uint8
		want     string
	}{
		{name: "nil", value: nil, decimals: 18, want: "0"},
		{name: "zero", value: big.NewInt(0), decimals: 18, want: "0"},
		{name: "whole", value: big.NewInt(1500000), decimals: 6, want: "1.5"},
		{name: "fraction", value: big.NewInt(1), decimals: 6, want: "0.000001"},
		{name: "no decimals", value: big.NewInt(42), decimals: 0, want: "42"},
	}

	for _, tt := range tests {
		if got := FormatUnits(tt.value, tt.decimals); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
