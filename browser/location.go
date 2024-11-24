package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
}

func getLocationFromIP() (*GeoLocation, error) {
	resp, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var result struct {
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		City    string  `json:"city"`
		Country string  `json:"country"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &GeoLocation{
		Latitude:  result.Lat,
		Longitude: result.Lon,
		City:      result.City,
		Country:   result.Country,
	}, nil
}
