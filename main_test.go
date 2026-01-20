package main

import (
	"math"
	"testing"
)

func TestParseYAML(t *testing.T) {
}

func TestCalculateOnCallEUR(t *testing.T) {
	// Given: OnCallNOK = 10000, rate = 11.5
	// a = 10000 / 11.5 = 869.57
	// a * 0.12 = 104.35
	// a + (a * 0.12) = 973.91
	nokAmount := 10000
	rate := 11.5
	expected := 973.91

	result := CalculateOnCallEUR(nokAmount, rate)

	// Round to 2 decimal places for comparison
	rounded := math.Round(result*100) / 100

	if rounded != expected {
		t.Errorf("CalculateOnCallEUR(%d, %f) = %f (rounded: %f), want %f",
			nokAmount, rate, result, rounded, expected)
	}
}
