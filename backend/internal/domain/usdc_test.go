package domain_test

import (
	"math/big"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

func TestFormatUSDC(t *testing.T) {
	for in, want := range map[string]string{
		"0":         "0.000000",
		"1":         "0.000001",
		"5000":      "0.005000",
		"999999":    "0.999999",
		"1000000":   "1.000000",
		"696500000": "696.500000",
		"340282366920938463463374607431768211455": "340282366920938463463374607431768.211455",
	} {
		n, ok := new(big.Int).SetString(in, 10)
		if !ok {
			t.Fatal(in)
		}
		if got := domain.FormatUSDC(n); got != want {
			t.Errorf("FormatUSDC(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestParseBaseUnits(t *testing.T) {
	if n, err := domain.ParseBaseUnits("007"); err != nil || n.Int64() != 7 {
		t.Errorf("ParseBaseUnits(007) = %v, %v", n, err)
	}
	for _, bad := range []string{"", "-1", "+1", "1.5", "1e6", " 1", "abc"} {
		if _, err := domain.ParseBaseUnits(bad); err == nil {
			t.Errorf("ParseBaseUnits(%q) accepted", bad)
		}
	}
}
