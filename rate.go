package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type RateQuery struct {
	Data RateData `json:"data"`
}

type RateData struct {
	DataSets []RateDataSet `json:"dataSets"`
}

type RateDataSet struct {
	Series RateDataSetSeries `json:"series"`
}

type RateDataSetSeries struct {
	Entry SeriesEntry `json:"0:0:0:0"`
}

type SeriesEntry struct {
	Observations map[string][]string `json:"observations"`
}

var httpClient http.Client

func GetRates() (RateData, error) {
	// TODO: allow modifying number of observations (should be atleast 30)
	rateAPI := "https://data.norges-bank.no/api/data/EXR/B.EUR.NOK.SP?format=sdmx-json&lastNObservations=1&locale=no"

	req, err := http.NewRequest(http.MethodGet, rateAPI, http.NoBody)
	if err != nil {
		return RateData{}, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return RateData{}, err
	}

	defer func() {
		_ = res.Body.Close()
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return RateData{}, err
	}

	if res.StatusCode != http.StatusOK {
		return RateData{}, errors.New("rate api did not respond with 200 OK")
	}

	var rateQuery RateQuery

	err = json.Unmarshal(body, &rateQuery)
	if err != nil {
		return RateData{}, err
	}

	return rateQuery.Data, nil
}
