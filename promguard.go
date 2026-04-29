package promguard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.k6.io/k6/js/modules"
)

func init() {
	modules.Register("k6/x/promguard", new(PromGuard))
}

type PromGuard struct{}

type Config struct {
	URL             string  `json:"url"`
	Query           string  `json:"query"`
	Threshold       float64 `json:"threshold"`
	DurationSeconds int     `json:"durationSeconds"`
	IntervalSeconds int     `json:"intervalSeconds"`
	Username        string  `json:"username"`
	Password        string  `json:"password"`
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p *PromGuard) Start(config Config) {
	if config.IntervalSeconds <= 0 {
		config.IntervalSeconds = 5
	}
	if config.DurationSeconds <= 0 {
		config.DurationSeconds = 60
	}

	go monitor(config)
}

func monitor(cfg Config) {
	ticker := time.NewTicker(time.Duration(cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	var violationStart *time.Time

	for range ticker.C {
		value, err := queryPrometheus(cfg)
		if err != nil {
			fmt.Println("Prometheus query error:", err)
			fmt.Println("Prometheus value:", value)
			continue
		}

		fmt.Println("Prometheus value:", value)

		if value >= cfg.Threshold {
			if violationStart == nil {
				now := time.Now()
				violationStart = &now
			} else {
				elapsed := time.Since(*violationStart)
				if elapsed.Seconds() >= float64(cfg.DurationSeconds) {
					fmt.Println("Threshold exceeded for duration. Stopping test.")
					stopK6()
				}
			}
		} else {
			violationStart = nil
		}
	}
}

func queryPrometheus(cfg Config) (float64, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/api/v1/query?query=%s", cfg.URL, url.QueryEscape(cfg.Query)),
		nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return 0, fmt.Errorf("bad json: %s", string(body))
	}

	if pr.Status != "success" {
		return 0, fmt.Errorf("prom error: %s", string(body))
	}

	if len(pr.Data.Result) == 0 {
		return 0, fmt.Errorf("empty result")
	}

	valStr := pr.Data.Result[0].Value[1].(string)

	var value float64
	fmt.Sscanf(valStr, "%f", &value)

	return value, nil
}
func stopK6() {
	panic("k6 stopped by promguard extension")
}
