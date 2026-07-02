package utils

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestRoundToTick(t *testing.T) {
	cases := []struct {
		name string
		x    string
		tick string
		want string
	}{
		{"already on grid", "0.79", "0.0025", "0.79"},
		{"on grid 4dp", "0.7925", "0.0025", "0.7925"},
		{"round down", "0.791", "0.0025", "0.79"},
		{"round up", "0.7913", "0.0025", "0.7925"},
		{"midpoint half away from zero", "0.79125", "0.0025", "0.7925"},
		{"min bound", "0.0025", "0.0025", "0.0025"},
		{"max bound", "0.9975", "0.0025", "0.9975"},
		{"power-of-ten tick 0.01", "0.555", "0.01", "0.56"},
		{"power-of-ten tick 0.001", "0.5549", "0.001", "0.555"},
		{"power-of-ten tick 0.1", "0.55", "0.1", "0.6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RoundToTick(d(c.x), d(c.tick))
			if !got.Equal(d(c.want)) {
				t.Fatalf("RoundToTick(%s, %s) = %s, want %s", c.x, c.tick, got, c.want)
			}
		})
	}
}
