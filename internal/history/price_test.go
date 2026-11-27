package history

import (
	"math"
	"testing"
)

func TestCostUsesTheModelsPrices(t *testing.T) {
	// 1M of each kind on Opus 5.5: 4 + 5 (write 5m) + 8 (write 1h) + 0.20 (read) + 20 (output)
	got, ok := cost("claude-opus-5-5", 1e6, 1e6, 1e6, 1e6, 1e6)
	if !ok || math.Abs(got-37.2) > 1e-9 {
		t.Fatalf("%v %v", got, ok)
	}
	if got, _ := cost("claude-opus-5", 1e6, 0, 0, 0, 0); got != 5 { // not taken for Opus 5.5
		t.Fatalf("opus 5: %v", got)
	}
	if _, ok := cost("<synthetic>", 1, 0, 0, 0, 0); ok {
		t.Fatal("an unknown model has no price")
	}
}
