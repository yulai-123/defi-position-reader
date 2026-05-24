package evm

import (
	"math/big"
	"strings"
)

func FormatUnits(value *big.Int, decimals uint8) string {
	if value == nil {
		return "0"
	}
	if value.Sign() == 0 {
		return "0"
	}
	if decimals == 0 {
		return value.String()
	}

	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	integer := new(big.Int).Quo(new(big.Int).Set(value), base)
	fraction := new(big.Int).Mod(new(big.Int).Set(value), base)
	if fraction.Sign() == 0 {
		return integer.String()
	}

	frac := fraction.String()
	if len(frac) < int(decimals) {
		frac = strings.Repeat("0", int(decimals)-len(frac)) + frac
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return integer.String()
	}
	return integer.String() + "." + frac
}
