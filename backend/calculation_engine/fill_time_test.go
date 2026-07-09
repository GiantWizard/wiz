package main

import (
	"math"
	"testing"
)

func TestCalculateInstasellFillTime(t *testing.T) {
	cases := []struct {
		name         string
		qty          float64
		buyMoving    float64
		wantTime     float64
		wantErr      bool
		wantInfinite bool
	}{
		{"zero quantity is instant", 0, 1000, 0, false, false},
		{"negative quantity is instant", -5, 1000, 0, false, false},
		{"no buy demand is infinite", 100, 0, 0, true, true},
		{"negative buy moving week is infinite", 100, -50, 0, true, true},
		{
			// buyRatePerSecond = 604800/604800 = 1/s, so 100 units take 100s
			"normal case divides qty by rate", 100, 604800, 100, false, false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			product := HypixelProduct{
				ProductID:   "TEST_ITEM",
				QuickStatus: QuickStatus{BuyMovingWeek: c.buyMoving},
			}
			got, err := calculateInstasellFillTime(c.qty, product)

			if c.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantInfinite {
				if !math.IsInf(got, 1) {
					t.Fatalf("expected +Inf, got %v", got)
				}
				return
			}
			if math.Abs(got-c.wantTime) > 1e-6 {
				t.Fatalf("expected %v seconds, got %v", c.wantTime, got)
			}
		})
	}
}

func TestCalculateBuyOrderFillTime_ZeroQuantity(t *testing.T) {
	fillTime, rr, err := calculateBuyOrderFillTime("TEST_ITEM", 0, ProductMetrics{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fillTime != 0 {
		t.Fatalf("expected 0 fill time for zero quantity, got %v", fillTime)
	}
	if !math.IsNaN(rr) {
		t.Fatalf("expected NaN RR for zero quantity, got %v", rr)
	}
}

func TestCalculateBuyOrderFillTime_PositiveNetFlow(t *testing.T) {
	// sell_size * sell_freq > order_size * order_freq => positive net flow branch.
	// deltaNetFlow = (10*5) - (2*2) = 50 - 4 = 46
	// fillTime = (20 * qty) / deltaNetFlow = (20*46)/46 = 20
	metrics := ProductMetrics{SellSize: 10, SellFrequency: 5, OrderSize: 2, OrderFrequency: 2}
	fillTime, rr, err := calculateBuyOrderFillTime("TEST_ITEM", 46, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(fillTime-20.0) > 1e-6 {
		t.Fatalf("expected fill time 20s, got %v", fillTime)
	}
	if math.IsNaN(rr) || math.IsInf(rr, 0) {
		t.Fatalf("expected a finite RR, got %v", rr)
	}
}

func TestCalculateBuyOrderFillTime_NoOrderFlow(t *testing.T) {
	// Zero order frequency and non-positive net flow => infinite fill time, non-nil error.
	metrics := ProductMetrics{SellSize: 0, SellFrequency: 0, OrderSize: 0, OrderFrequency: 0}
	fillTime, _, err := calculateBuyOrderFillTime("TEST_ITEM", 10, metrics)
	if err == nil {
		t.Fatalf("expected an error when there's no order flow at all")
	}
	if !math.IsInf(fillTime, 1) {
		t.Fatalf("expected +Inf fill time, got %v", fillTime)
	}
}
