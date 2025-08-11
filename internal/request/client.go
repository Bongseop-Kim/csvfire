package request

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"csvfire/internal/config"
)

// Client handles HTTP requests with retry logic and proxy support
type Client struct {
	requestConfig *config.RequestConfig
	baseClient    *http.Client
	maxRetries    int
	timeout       time.Duration
}

// RequestResult holds the result of an HTTP request
type RequestResult struct {
	StatusCode      int                    `json:"status_code"`
	Success         bool                   `json:"success"`
	LatencyMs       int64                  `json:"latency_ms"`
	Retries         int                    `json:"retries"`
	ErrorCategory   string                 `json:"error_category,omitempty"`
	ErrorDetail     string                 `json:"error_detail,omitempty"`
	ResponsePreview string                 `json:"response_preview,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	RequestID       string                 `json:"request_id"`
}

// NewClient creates a new HTTP client
func NewClient(requestConfig *config.RequestConfig, timeout time.Duration) *Client {
	return &Client{
		requestConfig: requestConfig,
		baseClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries: 3,
		timeout:    timeout,
	}
}

// SetMaxRetries sets the maximum number of retries
func (c *Client) SetMaxRetries(maxRetries int) {
	c.maxRetries = maxRetries
}

// Execute executes an HTTP request with retry logic
func (c *Client) Execute(ctx context.Context, requestData *RequestData, requestID string) *RequestResult {
	result := &RequestResult{
		RequestID: requestID,
		Headers:   make(map[string]string),
	}

	startTime := time.Now()
	
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		result.Retries = attempt

		// Create HTTP client with proxy if specified
		client := c.createClientWithProxy(requestData.Proxy)
		
		// Execute the request
		statusCode, responseBody, headers, err := c.executeRequest(ctx, client, requestData)
		
		result.StatusCode = statusCode
		result.LatencyMs = time.Since(startTime).Milliseconds()
		
		if headers != nil {
			for k, v := range headers {
				if len(v) > 0 {
					result.Headers[k] = v[0]
				}
			}
		}

		if err != nil {
			lastErr = err
			result.ErrorCategory = categorizeError(err)
			result.ErrorDetail = err.Error()

			// Only retry on network errors or 5xx status codes
			if !shouldRetry(err, statusCode) {
				break
			}

			// Don't sleep on the last attempt
			if attempt < c.maxRetries {
				backoffDelay := c.calculateBackoff(attempt)
				select {
				case <-ctx.Done():
					return result
				case <-time.After(backoffDelay):
					continue
				}
			}
		} else {
			// Success case
			result.Success = c.requestConfig.IsSuccessStatus(statusCode)
			result.ResponsePreview = truncateResponse(responseBody)
			
			// Check response JSON conditions if specified
			if result.Success && len(c.requestConfig.Success.ResponseKeys) > 0 {
				result.Success = c.checkResponseConditions(responseBody)
			}
			
			break
		}
	}

	// If we've exhausted retries and still have an error
	if lastErr != nil && result.ErrorCategory == "" {
		result.ErrorCategory = categorizeError(lastErr)
		result.ErrorDetail = lastErr.Error()
	}

	return result
}

// executeRequest executes a single HTTP request
func (c *Client) executeRequest(ctx context.Context, client *http.Client, requestData *RequestData) (int, string, http.Header, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, requestData.Method, requestData.URL, strings.NewReader(requestData.Body))
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range requestData.Headers {
		req.Header.Set(key, value)
	}

	// Set default headers if not specified
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "csvfire/1.0")
	}
	
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate")
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", resp.Header, fmt.Errorf("failed to read response body: %w", err)
	}

	return resp.StatusCode, string(body), resp.Header, nil
}

// createClientWithProxy creates an HTTP client with optional proxy
func (c *Client) createClientWithProxy(proxyURL string) *http.Client {
	client := &http.Client{
		Timeout: c.timeout,
	}

	if proxyURL != "" {
		if parsedProxy, err := url.Parse(proxyURL); err == nil {
			transport := &http.Transport{
				Proxy: http.ProxyURL(parsedProxy),
			}
			client.Transport = transport
		}
	}

	return client
}

// calculateBackoff calculates exponential backoff with jitter
func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Base delay of 1 second
	baseDelay := time.Second
	
	// Exponential backoff: 1s, 2s, 4s, 8s, etc.
	delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
	
	// Add jitter (±25%)
	jitter := time.Duration(rand.Float64() * 0.5 * float64(delay))
	if rand.Float64() < 0.5 {
		delay -= jitter
	} else {
		delay += jitter
	}
	
	// Cap at 30 seconds
	if delay > 30*time.Second {
		delay = 30*time.Second
	}
	
	return delay
}

// shouldRetry determines if a request should be retried
func shouldRetry(err error, statusCode int) bool {
	// Retry on network errors (when statusCode is 0)
	if statusCode == 0 {
		return true
	}
	
	// Retry on 5xx server errors
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	
	// Retry on 429 (Too Many Requests)
	if statusCode == 429 {
		return true
	}
	
	return false
}

// categorizeError categorizes errors for logging
func categorizeError(err error) string {
	if err == nil {
		return ""
	}
	
	errStr := err.Error()
	
	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection refused"):
		return "connection_refused"
	case strings.Contains(errStr, "no such host"):
		return "dns_error"
	case strings.Contains(errStr, "context canceled"):
		return "canceled"
	case strings.Contains(errStr, "context deadline exceeded"):
		return "timeout"
	default:
		return "unknown"
	}
}

// truncateResponse truncates response body for preview
func truncateResponse(body string) string {
	maxLen := 200
	if len(body) <= maxLen {
		return body
	}
	
	return body[:maxLen] + "..."
}

// checkResponseConditions checks JSON response conditions
func (c *Client) checkResponseConditions(responseBody string) bool {
	if len(c.requestConfig.Success.ResponseKeys) == 0 {
		return true
	}
	
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &jsonData); err != nil {
		return false // Can't parse JSON, consider as failure
	}
	
	// Check all required key-value pairs
	for key, expectedValue := range c.requestConfig.Success.ResponseKeys {
		if actualValue, exists := jsonData[key]; !exists {
			return false
		} else if fmt.Sprintf("%v", actualValue) != expectedValue {
			return false
		}
	}
	
	return true
} 