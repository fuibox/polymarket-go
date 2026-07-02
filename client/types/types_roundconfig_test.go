package types

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestGetRoundConfig00025(t *testing.T) {
	rc := GetRoundConfig(TickSize00025)
	if rc == nil {
		t.Fatal("GetRoundConfig(\"0.0025\") = nil, want non-nil")
	}
	if rc.Price != 4 || rc.Size != 2 || rc.Amount != 6 {
		t.Fatalf("RoundConfig = %+v, want {Price:4 Size:2 Amount:6}", rc)
	}
	if !rc.Tick.Equal(decimal.New(25, -4)) {
		t.Fatalf("Tick = %s, want 0.0025", rc.Tick)
	}
}

func TestGetRoundConfigUnknownStillNil(t *testing.T) {
	if rc := GetRoundConfig(TickSize("0.005")); rc != nil {
		t.Fatalf("GetRoundConfig(\"0.005\") = %+v, want nil", rc)
	}
}

func TestRoundingConfigAllEntriesHaveTick(t *testing.T) {
	want := map[TickSize]decimal.Decimal{
		TickSize01:    decimal.New(1, -1),
		TickSize001:   decimal.New(1, -2),
		TickSize0001:  decimal.New(1, -3),
		TickSize00001: decimal.New(1, -4),
		TickSize00025: decimal.New(25, -4),
	}
	if len(RoundingConfig) != len(want) {
		t.Fatalf("RoundingConfig has %d entries, want %d", len(RoundingConfig), len(want))
	}
	for ts, tick := range want {
		rc, ok := RoundingConfig[ts]
		if !ok {
			t.Fatalf("RoundingConfig missing entry for %q", ts)
		}
		if !rc.Tick.Equal(tick) {
			t.Fatalf("RoundingConfig[%q].Tick = %s, want %s", ts, rc.Tick, tick)
		}
	}
}
