package main

import (
	"testing"
)

func TestGetDailyRates(t *testing.T) {
}

func TestFindRateOn15th(t *testing.T) {
	rates, _ := GetDailyRates(365) // TODO do some error assertions

	FindRateOn15th(rates, "January")
}
