package order_builder

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	"github.com/fuibox/polymarket-go/client/types"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func rc(ts types.TickSize) types.RoundConfig {
	r := types.GetRoundConfig(ts)
	if r == nil {
		panic("missing round config for " + string(ts))
	}
	return *r
}

func clobPartialOptions(ts *types.TickSize, negRisk *bool) clob_types.PartialCreateOrderOptions {
	return clob_types.PartialCreateOrderOptions{TickSize: ts, NegRisk: negRisk}
}

// 限价单矩阵：0.0025 新档 + 既有 4 档金样例（金样例值 = 改造前 RoundNormal 行为，锁死零回归）。
func TestGetOrderAmounts(t *testing.T) {
	b := &OrderBuilder{}
	cases := []struct {
		name      string
		tick      types.TickSize
		side      types.Side
		size      string
		price     string
		wantMaker string
		wantTaker string
	}{
		// —— 0.0025 新档 ——
		{"00025 buy on-grid price", types.TickSize00025, types.SideBuy, "379.74", "0.79", "299994600", "379740000"},
		{"00025 buy price snaps up", types.TickSize00025, types.SideBuy, "100", "0.7913", "79250000", "100000000"},
		{"00025 buy price snaps down", types.TickSize00025, types.SideBuy, "100", "0.791", "79000000", "100000000"},
		{"00025 sell price snaps down", types.TickSize00025, types.SideSell, "379.74", "0.791", "379740000", "299994600"},
		{"00025 buy min price", types.TickSize00025, types.SideBuy, "100", "0.0025", "250000", "100000000"},
		{"00025 buy max price", types.TickSize00025, types.SideBuy, "100", "0.9975", "99750000", "100000000"},
		// —— 既有档金样例（与改造前输出一致）——
		{"01 buy golden", types.TickSize01, types.SideBuy, "10.567", "0.55", "6336000", "10560000"},
		{"001 buy golden", types.TickSize001, types.SideBuy, "100", "0.555", "56000000", "100000000"},
		{"0001 sell golden", types.TickSize0001, types.SideSell, "200", "0.4567", "200000000", "91400000"},
		{"00001 buy golden", types.TickSize00001, types.SideBuy, "100", "0.12345", "12350000", "100000000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sideInt, maker, taker, err := b.GetOrderAmounts(c.side, d(c.size), d(c.price), rc(c.tick))
			if err != nil {
				t.Fatalf("GetOrderAmounts err = %v", err)
			}
			if sideInt != c.side.Int() {
				t.Fatalf("sideInt = %d, want %d", sideInt, c.side.Int())
			}
			if maker != c.wantMaker || taker != c.wantTaker {
				t.Fatalf("maker/taker = %s/%s, want %s/%s", maker, taker, c.wantMaker, c.wantTaker)
			}
		})
	}
}

// 市价单矩阵：0.0025 新档 + 既有档金样例。
func TestGetMarketOrderAmounts(t *testing.T) {
	b := &OrderBuilder{}
	cases := []struct {
		name      string
		tick      types.TickSize
		side      types.Side
		amount    string
		price     string
		wantMaker string
		wantTaker string
	}{
		{"00025 market buy", types.TickSize00025, types.SideBuy, "300", "0.79", "300000000", "379746835"},
		{"00025 market sell snaps down", types.TickSize00025, types.SideSell, "50", "0.791", "50000000", "39500000"},
		{"00001 market buy golden", types.TickSize00001, types.SideBuy, "100", "0.12345", "100000000", "809716599"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sideInt, maker, taker, err := b.GetMarketOrderAmounts(c.side, d(c.amount), d(c.price), rc(c.tick))
			if err != nil {
				t.Fatalf("GetMarketOrderAmounts err = %v", err)
			}
			if sideInt != c.side.Int() {
				t.Fatalf("sideInt = %d, want %d", sideInt, c.side.Int())
			}
			if maker != c.wantMaker || taker != c.wantTaker {
				t.Fatalf("maker/taker = %s/%s, want %s/%s", maker, taker, c.wantMaker, c.wantTaker)
			}
		})
	}
}

// 零值 Tick（手工构造的旧 RoundConfig）必须回退「保留 N 位小数」旧语义。
func TestGetOrderAmountsLegacyRoundConfigFallback(t *testing.T) {
	b := &OrderBuilder{}
	legacy := types.RoundConfig{Price: 2, Size: 2, Amount: 4} // Tick 为零值
	_, maker, taker, err := b.GetOrderAmounts(types.SideBuy, d("100"), d("0.555"), legacy)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// RoundNormal(0.555, 2) = 0.56 → maker = 100*0.56
	if maker != "56000000" || taker != "100000000" {
		t.Fatalf("maker/taker = %s/%s, want 56000000/100000000", maker, taker)
	}
}

// 未知档位仍必须报 get round config error（Task 4 的后端映射依赖该文案）。
func TestValidateAndGetRoundConfigUnknownTick(t *testing.T) {
	unknown := types.TickSize("0.005")
	negRisk := false
	_, err := validateAndGetRoundConfig(clobPartialOptions(&unknown, &negRisk))
	if err == nil || err.Error() != "get round config error" {
		t.Fatalf("err = %v, want \"get round config error\"", err)
	}
}
