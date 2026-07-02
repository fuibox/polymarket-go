package clob

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/fuibox/polymarket-go/client/types"
)

func TestPriceValid00025(t *testing.T) {
	c := &ClobClient{}
	cases := []struct {
		price   string
		wantErr bool
	}{
		{"0.0025", false}, // min = tick
		{"0.9975", false}, // max = 1 - tick
		{"0.79", false},
		{"0.001", true},  // < min
		{"0.998", true},  // > max
	}
	for _, cs := range cases {
		err := c.priceValid(decimal.RequireFromString(cs.price), types.TickSize00025)
		if (err != nil) != cs.wantErr {
			t.Fatalf("priceValid(%s, 0.0025) err = %v, wantErr = %v", cs.price, err, cs.wantErr)
		}
	}
}
