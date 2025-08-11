package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RequestConfig represents the HTTP request configuration
type RequestConfig struct {
	Method   string                 `yaml:"method"`
	URL      string                 `yaml:"url"`
	Headers  map[string]string      `yaml:"headers"`
	Body     string                 `yaml:"body"`
	Proxy    string                 `yaml:"proxy,omitempty"`
	Success  SuccessCondition       `yaml:"success"`
	Timeout  string                 `yaml:"timeout,omitempty"`
}

// SuccessCondition defines conditions for successful requests
type SuccessCondition struct {
	StatusIn     []int             `yaml:"status_in"`
	ResponseKeys map[string]string `yaml:"response_keys,omitempty"`
}

// LoadRequestConfig loads and parses a request configuration file
func LoadRequestConfig(filename string) (*RequestConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read request config file: %w", err)
	}

	var config RequestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse request config YAML: %w", err)
	}

	// Validate request config
	if err := validateRequestConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid request config: %w", err)
	}

	return &config, nil
}

// validateRequestConfig performs basic validation on the request configuration
func validateRequestConfig(config *RequestConfig) error {
	if config.Method == "" {
		return fmt.Errorf("method is required")
	}

	if config.URL == "" {
		return fmt.Errorf("url is required")
	}

	if len(config.Success.StatusIn) == 0 {
		// Default to 200-299 status codes
		config.Success.StatusIn = []int{200, 201, 202, 203, 204, 205, 206, 207, 208, 226}
	}

	return nil
}

// IsSuccessStatus checks if the given status code is considered successful
func (rc *RequestConfig) IsSuccessStatus(statusCode int) bool {
	for _, code := range rc.Success.StatusIn {
		if code == statusCode {
			return true
		}
	}
	return false
} 