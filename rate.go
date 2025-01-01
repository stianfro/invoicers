package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type RateQuery struct {
	Data RateData `json:"data"`
}

type RateData struct {
	DataSets  []RateDataSet `json:"dataSets"`
	Structure RateStructure `json:"structure"`
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

func GetDailyRates(observationCount int) (RateData, error) {
	rateAPI := fmt.Sprintf("https://data.norges-bank.no/api/data/EXR/B.EUR.NOK.SP?format=sdmx-json&lastNObservations=%d&locale=no", observationCount)

	req, err := http.NewRequest(http.MethodGet, rateAPI, http.NoBody)
	if err != nil {
		return RateData{}, err
	}

	httpClient := http.Client{}

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

type RateStructure struct {
	Dimensions RateDimensions `json:"dimensions"`
}

type RateDimensions struct {
	Observation []RateDimensionsObservation `json:"observation"`
}

type RateDimensionsObservation struct {
	ID     string             `json:"id"`
	Values []ObservationValue `json:"values"`
}

type ObservationValue struct {
	Start string `json:"start"`
	End   string `json:"end"`
	ID    string `json:"id"`
	Name  string `json:"name"`
}

// loop over k,v in series entry observation
// use k to access matching entry in dimensions observation
func FindRateOn15th(data RateData) (string, error) {
	if len(data.DataSets) == 0 {
		return "", errors.New("rate dataset was empty")
	}

	rateObservations := data.DataSets[0].Series.Entry.Observations
	dimensionsObservation := data.Structure.Dimensions.Observation

	dayRate := make(map[int]string)

	for key, rate := range rateObservations {
		keyInt, err := strconv.Atoi(key)
		if err != nil {
			return "", err
		}

		if len(dimensionsObservation) == 0 {
			return "", errors.New("dimension observations were empty")
		}

		observationValues := dimensionsObservation[0].Values[keyInt]

		observationDate, err := ParseDate(observationValues.Start)
		if err != nil {
			return "", err
		}

		// now := time.Now()
		// if observationDate.Month().String() != now.Month().String() {

		observationMonthString := observationDate.Month().String()

		// TODO: add better support for specifying month
		if observationMonthString != "December" {
			continue
		}

		dayRate[observationDate.Day()] = rate[0]
	}

	return DecideDay(dayRate), nil
}

func DecideDay(dayRate map[int]string) string {
	if dayRate[15] != "" {
		return dayRate[15]
	}

	if dayRate[14] != "" {
		return dayRate[14]
	}

	return dayRate[16]
}

func ParseDate(date string) (time.Time, error) {
	layout := "2006-01-02T15:04:05"

	t, err := time.Parse(layout, date)
	if err != nil {
		return time.Time{}, err
	}

	return t, nil
}
