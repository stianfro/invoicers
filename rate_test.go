package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDailyRates(t *testing.T) {
}

func TestFindRateOn15th(t *testing.T) {
	rates, _ := GetDailyRates(30) // TODO do some error assertions

	_, _ = FindRateOn15th(rates, "December")
}

func TestDecideDay(t *testing.T) {
	tests := []struct {
		name  string
		input map[int]string
		want  string
	}{
		{
			name: "map with 15th",
			input: map[int]string{
				14: "5.0",
				15: "10.0",
				16: "20.0",
			},
			want: "10.0",
		},
		{
			name: "map without 15th, with 14 (preferred) and 16",
			input: map[int]string{
				14: "5.0",
				16: "20.0",
			},
			want: "5.0",
		},
		{
			name: "map without 15th, with 16",
			input: map[int]string{
				16: "20.0",
			},
			want: "20.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideDay(tt.input)
			assert.Equal(t, got, tt.want)
		})
	}
}
