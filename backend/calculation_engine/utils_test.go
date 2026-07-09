package main

import "testing"

func TestBAZAAR_ID_Normalization(t *testing.T) {
	cases := map[string]string{
		"  oak_log ":  "OAK_LOG", // whitespace + case, no mapping needed
		"LOG":         "OAK_LOG", // legacy numeric ID mapping
		"log:1":       "SPRUCE_LOG",
		"INK_SACK:4":  "LAPIS_LAZULI",
		"ENDER_STONE": "END_STONE",
		"DIAMOND":     "DIAMOND", // no mapping, just uppercased/trimmed
		"":            "",
	}
	for input, want := range cases {
		if got := BAZAAR_ID(input); got != want {
			t.Errorf("BAZAAR_ID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAggregateCells(t *testing.T) {
	cells := map[string]string{
		"A1": "OAK_LOG:2",
		"A2": "oak_log:3", // same normalized ID, different case, should sum with A1
		"B1": "DIAMOND",   // no amount suffix, defaults to 1
		"C3": "",          // empty cell, skipped
	}
	got, err := aggregateCells(cells)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["OAK_LOG"] != 5 {
		t.Errorf("expected OAK_LOG total 5, got %v", got["OAK_LOG"])
	}
	if got["DIAMOND"] != 1 {
		t.Errorf("expected DIAMOND total 1, got %v", got["DIAMOND"])
	}
	if len(got) != 2 {
		t.Errorf("expected 2 distinct ingredients, got %d: %+v", len(got), got)
	}
}

func TestAggregateCells_InvalidAmountFallsBackToOne(t *testing.T) {
	cells := map[string]string{"A1": "DIAMOND:not_a_number"}
	got, err := aggregateCells(cells)
	if err == nil {
		t.Fatalf("expected an error to be reported for the invalid amount")
	}
	if got["DIAMOND"] != 1 {
		t.Errorf("expected fallback amount of 1 for invalid amount string, got %v", got["DIAMOND"])
	}
}

func TestMapsAreEqual(t *testing.T) {
	a := map[string]float64{"X": 1.0, "Y": 2.0000000001}
	b := map[string]float64{"X": 1.0, "Y": 2.0}
	if !mapsAreEqual(a, b) {
		t.Errorf("expected maps within tolerance to be equal")
	}

	c := map[string]float64{"X": 1.0, "Y": 3.0}
	if mapsAreEqual(a, c) {
		t.Errorf("expected maps with a real difference to not be equal")
	}

	d := map[string]float64{"X": 1.0}
	if mapsAreEqual(a, d) {
		t.Errorf("expected maps of different lengths to not be equal")
	}
}
