package domain

import (
	"fmt"
	"math/big"
	"strings"
)

// USDCDecimals is the number of decimals of USDC: an amount in base units
// is divided by 10^6 to give USDC.
const USDCDecimals = 6

// ParseBaseUnits parses a non-negative amount in base units, as the contract
// writes it: decimal digits only.
func ParseBaseUnits(s string) (*big.Int, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return nil, fmt.Errorf("amount %q is not a decimal number of base units", s)
	}
	n, _ := new(big.Int).SetString(s, 10)
	return n, nil
}

// FormatUSDC writes an amount in base units as USDC with all six decimals,
// e.g. 696500000 as "696.500000". The amount must not be negative.
func FormatUSDC(amount *big.Int) string {
	digits := amount.String()
	if len(digits) <= USDCDecimals {
		digits = strings.Repeat("0", USDCDecimals-len(digits)+1) + digits
	}
	cut := len(digits) - USDCDecimals
	return digits[:cut] + "." + digits[cut:]
}
