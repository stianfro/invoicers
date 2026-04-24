package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
)

const defaultBaseURL = "https://data.norges-bank.no/api/data/EXR/B.EUR.NOK.SP"

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

func (c *Client) RateForMonth(ctx context.Context, month time.Time) (*invoice.FXSnapshot, error) {
	month = invoice.MonthStart(month)
	end := month.AddDate(0, 1, -1)

	reqURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse fx base url: %w", err)
	}

	query := reqURL.Query()
	query.Set("format", "sdmx-json")
	query.Set("locale", "no")
	query.Set("startPeriod", month.Format("2006-01-02"))
	query.Set("endPeriod", end.Format("2006-01-02"))
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build fx request: %w", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform fx request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fx api returned status %s", res.Status)
	}

	var payload rateQuery
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode fx response: %w", err)
	}

	observations, err := payload.toDayRates(month)
	if err != nil {
		return nil, err
	}

	best, err := SelectBestObservation(observations)
	if err != nil {
		return nil, err
	}

	observedAt := month.AddDate(0, 0, 14)
	if _, ok := observations[15]; !ok {
		if _, ok := observations[14]; ok {
			observedAt = month.AddDate(0, 0, 13)
		} else {
			observedAt = month.AddDate(0, 0, 15)
		}
	}

	return &invoice.FXSnapshot{
		Rate:       best,
		Currency:   "NOK",
		Base:       "EUR",
		ObservedAt: observedAt,
		Source:     reqURL.String(),
	}, nil
}

func SelectBestObservation(observations map[int]float64) (float64, error) {
	if rate, ok := observations[15]; ok {
		return rate, nil
	}
	if rate, ok := observations[14]; ok {
		return rate, nil
	}
	if rate, ok := observations[16]; ok {
		return rate, nil
	}
	return 0, fmt.Errorf("no 14th/15th/16th observation available")
}

type rateQuery struct {
	Data rateData `json:"data"`
}

type rateData struct {
	DataSets  []rateDataSet `json:"dataSets"`
	Structure rateStructure `json:"structure"`
}

type rateDataSet struct {
	Series map[string]rateSeries `json:"series"`
}

type rateSeries struct {
	Observations map[string][]string `json:"observations"`
}

type rateStructure struct {
	Dimensions rateDimensions `json:"dimensions"`
}

type rateDimensions struct {
	Observation []rateObservationDimension `json:"observation"`
}

type rateObservationDimension struct {
	Values []rateObservationValue `json:"values"`
}

type rateObservationValue struct {
	Start string `json:"start"`
}

func (q rateQuery) toDayRates(month time.Time) (map[int]float64, error) {
	if len(q.Data.DataSets) == 0 {
		return nil, fmt.Errorf("fx response contained no datasets")
	}
	if len(q.Data.Structure.Dimensions.Observation) == 0 {
		return nil, fmt.Errorf("fx response contained no observations")
	}

	dimensionValues := q.Data.Structure.Dimensions.Observation[0].Values
	dayRates := make(map[int]float64)

	for _, series := range q.Data.DataSets[0].Series {
		for key, values := range series.Observations {
			index, err := strconv.Atoi(key)
			if err != nil {
				return nil, fmt.Errorf("parse observation index: %w", err)
			}
			if index >= len(dimensionValues) || len(values) == 0 {
				continue
			}

			observedAt, err := time.Parse("2006-01-02T15:04:05", dimensionValues[index].Start)
			if err != nil {
				return nil, fmt.Errorf("parse observation date: %w", err)
			}
			if observedAt.Year() != month.Year() || observedAt.Month() != month.Month() {
				continue
			}

			rate, err := strconv.ParseFloat(values[0], 64)
			if err != nil {
				return nil, fmt.Errorf("parse observation value: %w", err)
			}
			dayRates[observedAt.Day()] = rate
		}
	}

	if len(dayRates) == 0 {
		return nil, fmt.Errorf("no rates found for %s", month.Format("2006-01"))
	}

	return dayRates, nil
}
