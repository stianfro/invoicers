package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSelectBestObservation(t *testing.T) {
	t.Parallel()

	observations := map[int]float64{
		14: 11.42,
		15: 11.50,
		16: 11.48,
	}

	got, err := SelectBestObservation(observations)
	if err != nil {
		t.Fatalf("SelectBestObservation returned error: %v", err)
	}

	if got != 11.50 {
		t.Fatalf("SelectBestObservation = %.2f, want %.2f", got, 11.50)
	}
}

func TestClientRateForMonth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"dataSets": [{
					"series": {
						"0:0:0:0": {
							"observations": {
								"0": ["11.42"],
								"1": ["11.50"],
								"2": ["11.48"]
							}
						}
					}
				}],
				"structure": {
					"dimensions": {
						"observation": [{
							"id": "TIME_PERIOD",
							"values": [
								{"start": "2026-04-14T00:00:00", "id": "0"},
								{"start": "2026-04-15T00:00:00", "id": "1"},
								{"start": "2026-04-16T00:00:00", "id": "2"}
							]
						}]
					}
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	got, err := client.RateForMonth(context.Background(), time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RateForMonth returned error: %v", err)
	}

	if got.Rate != 11.50 {
		t.Fatalf("RateForMonth rate = %.2f, want %.2f", got.Rate, 11.50)
	}

	if got.Source == "" {
		t.Fatalf("RateForMonth source = %q, want non-empty source", got.Source)
	}
}
