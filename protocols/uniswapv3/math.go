package uniswapv3

import (
	"fmt"
	"math/big"
)

const (
	minTick = -887272
	maxTick = 887272
)

var (
	q96        = new(big.Int).Lsh(big.NewInt(1), 96)
	q128       = new(big.Int).Lsh(big.NewInt(1), 128)
	uint256    = new(big.Int).Lsh(big.NewInt(1), 256)
	maxUint256 = new(big.Int).Sub(new(big.Int).Set(uint256), big.NewInt(1))
)

func sqrtRatioAtTick(tick int32) (*big.Int, error) {
	if tick < minTick || tick > maxTick {
		return nil, fmt.Errorf("tick %d outside supported range [%d,%d]", tick, minTick, maxTick)
	}
	absTick := int64(tick)
	if absTick < 0 {
		absTick = -absTick
	}

	ratio := hexBig("0x100000000000000000000000000000000")
	if absTick&0x1 != 0 {
		ratio = hexBig("0xfffcb933bd6fad37aa2d162d1a594001")
	}
	mulShift := func(mask int64, value string) {
		if absTick&mask != 0 {
			ratio.Mul(ratio, hexBig(value))
			ratio.Rsh(ratio, 128)
		}
	}
	mulShift(0x2, "0xfff97272373d413259a46990580e213a")
	mulShift(0x4, "0xfff2e50f5f656932ef12357cf3c7fdcc")
	mulShift(0x8, "0xffe5caca7e10e4e61c3624eaa0941cd0")
	mulShift(0x10, "0xffcb9843d60f6159c9db58835c926644")
	mulShift(0x20, "0xff973b41fa98c081472e6896dfb254c0")
	mulShift(0x40, "0xff2ea16466c96a3843ec78b326b52861")
	mulShift(0x80, "0xfe5dee046a99a2a811c461f1969c3053")
	mulShift(0x100, "0xfcbe86c7900a88aedcffc83b479aa3a4")
	mulShift(0x200, "0xf987a7253ac413176f2b074cf7815e54")
	mulShift(0x400, "0xf3392b0822b70005940c7a398e4b70f3")
	mulShift(0x800, "0xe7159475a2c29b7443b29c7fa6e889d9")
	mulShift(0x1000, "0xd097f3bdfd2022b8845ad8f792aa5825")
	mulShift(0x2000, "0xa9f746462d870fdf8a65dc1f90e061e5")
	mulShift(0x4000, "0x70d869a156d2a1b890bb3df62baf32f7")
	mulShift(0x8000, "0x31be135f97d08fd981231505542fcfa6")
	mulShift(0x10000, "0x9aa508b5b7a84e1c677de54f3e99bc9")
	mulShift(0x20000, "0x5d6af8dedb81196699c329225ee604")
	mulShift(0x40000, "0x2216e584f5fa1ea926041bedfe98")
	mulShift(0x80000, "0x48a170391f7dc42444e8fa2")

	if tick > 0 {
		ratio.Div(maxUint256, ratio)
	}

	remainderMask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 32), big.NewInt(1))
	remainder := new(big.Int).And(ratio, remainderMask)
	ratio.Rsh(ratio, 32)
	if remainder.Sign() > 0 {
		ratio.Add(ratio, big.NewInt(1))
	}
	return ratio, nil
}

func amountsForLiquidity(sqrtPriceX96, sqrtLowerX96, sqrtUpperX96, liquidity *big.Int) (*big.Int, *big.Int) {
	liquidity = zeroIfNil(liquidity)
	if liquidity.Sign() == 0 {
		return big.NewInt(0), big.NewInt(0)
	}
	if sqrtLowerX96.Cmp(sqrtUpperX96) > 0 {
		sqrtLowerX96, sqrtUpperX96 = sqrtUpperX96, sqrtLowerX96
	}
	switch {
	case sqrtPriceX96.Cmp(sqrtLowerX96) <= 0:
		return amount0ForLiquidity(sqrtLowerX96, sqrtUpperX96, liquidity), big.NewInt(0)
	case sqrtPriceX96.Cmp(sqrtUpperX96) < 0:
		return amount0ForLiquidity(sqrtPriceX96, sqrtUpperX96, liquidity),
			amount1ForLiquidity(sqrtLowerX96, sqrtPriceX96, liquidity)
	default:
		return big.NewInt(0), amount1ForLiquidity(sqrtLowerX96, sqrtUpperX96, liquidity)
	}
}

func amount0ForLiquidity(sqrtA, sqrtB, liquidity *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	diff := new(big.Int).Sub(sqrtB, sqrtA)
	numerator := new(big.Int).Mul(new(big.Int).Lsh(zeroIfNil(liquidity), 96), diff)
	if sqrtB.Sign() == 0 || sqrtA.Sign() == 0 {
		return big.NewInt(0)
	}
	numerator.Div(numerator, sqrtB)
	return numerator.Div(numerator, sqrtA)
}

func amount1ForLiquidity(sqrtA, sqrtB, liquidity *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	diff := new(big.Int).Sub(sqrtB, sqrtA)
	out := new(big.Int).Mul(zeroIfNil(liquidity), diff)
	return out.Div(out, q96)
}

func feeGrowthInside(currentTick, lowerTick, upperTick int32, global0, global1 *big.Int, lower, upper tickOutput) (*big.Int, *big.Int) {
	below0, below1 := lower.FeeGrowthOutside0X128, lower.FeeGrowthOutside1X128
	if currentTick < lowerTick {
		below0 = subUint256(global0, below0)
		below1 = subUint256(global1, below1)
	}

	above0, above1 := upper.FeeGrowthOutside0X128, upper.FeeGrowthOutside1X128
	if currentTick >= upperTick {
		above0 = subUint256(global0, above0)
		above1 = subUint256(global1, above1)
	}

	inside0 := subUint256(subUint256(global0, below0), above0)
	inside1 := subUint256(subUint256(global1, below1), above1)
	return inside0, inside1
}

func uncollectedFees(liquidity, inside0, inside1, insideLast0, insideLast1, tokensOwed0, tokensOwed1 *big.Int) (*big.Int, *big.Int) {
	fee0 := new(big.Int).Mul(zeroIfNil(liquidity), subUint256(inside0, insideLast0))
	fee0.Div(fee0, q128)
	fee0.Add(fee0, zeroIfNil(tokensOwed0))

	fee1 := new(big.Int).Mul(zeroIfNil(liquidity), subUint256(inside1, insideLast1))
	fee1.Div(fee1, q128)
	fee1.Add(fee1, zeroIfNil(tokensOwed1))
	return fee0, fee1
}

func subUint256(a, b *big.Int) *big.Int {
	out := new(big.Int).Sub(zeroIfNil(a), zeroIfNil(b))
	if out.Sign() < 0 {
		out.Add(out, uint256)
	}
	return out
}

func rangeStatus(currentTick, lowerTick, upperTick int32) string {
	switch {
	case currentTick < lowerTick:
		return "below-range"
	case currentTick >= upperTick:
		return "above-range"
	default:
		return "in-range"
	}
}

func hexBig(value string) *big.Int {
	out, ok := new(big.Int).SetString(stringsTrimHex(value), 16)
	if !ok {
		panic("invalid hex big int: " + value)
	}
	return out
}

func stringsTrimHex(value string) string {
	if len(value) >= 2 && value[:2] == "0x" {
		return value[2:]
	}
	return value
}
