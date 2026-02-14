package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WeatherTool struct {
	apiKey      string
	defaultZip  string
}

func NewWeatherTool(apiKey, defaultZip string) *WeatherTool {
	return &WeatherTool{
		apiKey:     apiKey,
		defaultZip: defaultZip,
	}
}

func (t *WeatherTool) Name() string {
	return "weather"
}

func (t *WeatherTool) Description() string {
	return "Get current weather for a location. Use ZIP code or city name. Defaults to home location if not specified."
}

func (t *WeatherTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"location": map[string]interface{}{
				"type":        "string",
				"description": "ZIP code (e.g., '33547') or city name (e.g., 'Tampa,FL'). Optional - defaults to home.",
			},
		},
		"required": []string{},
	}
}

func (t *WeatherTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	location, _ := args["location"].(string)
	if location == "" {
		location = t.defaultZip
	}

	// Build API URL
	var apiURL string
	if len(location) == 5 && isNumeric(location) {
		// ZIP code
		apiURL = fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?zip=%s,us&units=imperial&appid=%s",
			location, t.apiKey)
	} else {
		// City name
		apiURL = fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&units=imperial&appid=%s",
			location, t.apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("weather request failed: %v", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	if resp.StatusCode != 200 {
		return ErrorResult(fmt.Sprintf("weather API error: %s", string(body)))
	}

	var weather struct {
		Name string `json:"name"`
		Main struct {
			Temp     float64 `json:"temp"`
			Humidity int     `json:"humidity"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
	}

	if err := json.Unmarshal(body, &weather); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse weather: %v", err))
	}

	desc := "unknown"
	if len(weather.Weather) > 0 {
		desc = weather.Weather[0].Description
	}

	result := fmt.Sprintf("%s: %.0f°F, %d%% humidity, %s, wind %.0f mph",
		weather.Name, weather.Main.Temp, weather.Main.Humidity, desc, weather.Wind.Speed)

	return &ToolResult{
		ForLLM:  result,
		ForUser: result,
		IsError: false,
	}
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
