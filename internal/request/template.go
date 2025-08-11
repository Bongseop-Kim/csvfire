package request

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"csvfire/internal/config"
)

// TemplateRenderer handles request template rendering
type TemplateRenderer struct {
	requestConfig *config.RequestConfig
	urlTemplate   *template.Template
	bodyTemplate  *template.Template
	headerTemplates map[string]*template.Template
	proxyTemplate *template.Template
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer(requestConfig *config.RequestConfig) (*TemplateRenderer, error) {
	renderer := &TemplateRenderer{
		requestConfig:   requestConfig,
		headerTemplates: make(map[string]*template.Template),
	}

	// Create template functions
	funcMap := template.FuncMap{
		"dateFormat":  dateFormat,
		"toE164KR":    toE164KR,
		"mask":        mask,
		"hash":        hash,
		"upper":       strings.ToUpper,
		"lower":       strings.ToLower,
		"trim":        strings.TrimSpace,
	}

	// Parse URL template
	urlTmpl, err := template.New("url").Funcs(funcMap).Parse(requestConfig.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL template: %w", err)
	}
	renderer.urlTemplate = urlTmpl

	// Parse body template
	if requestConfig.Body != "" {
		bodyTmpl, err := template.New("body").Funcs(funcMap).Parse(requestConfig.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to parse body template: %w", err)
		}
		renderer.bodyTemplate = bodyTmpl
	}

	// Parse header templates
	for key, value := range requestConfig.Headers {
		headerTmpl, err := template.New("header_"+key).Funcs(funcMap).Parse(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse header template for %s: %w", key, err)
		}
		renderer.headerTemplates[key] = headerTmpl
	}

	// Parse proxy template
	if requestConfig.Proxy != "" {
		proxyTmpl, err := template.New("proxy").Funcs(funcMap).Parse(requestConfig.Proxy)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy template: %w", err)
		}
		renderer.proxyTemplate = proxyTmpl
	}

	return renderer, nil
}

// RequestData holds all data needed for rendering a request
type RequestData struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Proxy   string            `json:"proxy,omitempty"`
	Hash    string            `json:"hash"`
}

// Render renders the request template with the given data
func (tr *TemplateRenderer) Render(data map[string]string) (*RequestData, error) {
	result := &RequestData{
		Method:  tr.requestConfig.Method,
		Headers: make(map[string]string),
	}

	// Render URL
	var urlBuf bytes.Buffer
	if err := tr.urlTemplate.Execute(&urlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render URL: %w", err)
	}
	result.URL = urlBuf.String()

	// Render headers
	for key, tmpl := range tr.headerTemplates {
		var headerBuf bytes.Buffer
		if err := tmpl.Execute(&headerBuf, data); err != nil {
			return nil, fmt.Errorf("failed to render header %s: %w", key, err)
		}
		headerValue := headerBuf.String()
		if headerValue != "" { // Only include non-empty headers
			result.Headers[key] = headerValue
		}
	}

	// Render body
	if tr.bodyTemplate != nil {
		var bodyBuf bytes.Buffer
		if err := tr.bodyTemplate.Execute(&bodyBuf, data); err != nil {
			return nil, fmt.Errorf("failed to render body: %w", err)
		}
		result.Body = bodyBuf.String()
	}

	// Render proxy
	if tr.proxyTemplate != nil {
		var proxyBuf bytes.Buffer
		if err := tr.proxyTemplate.Execute(&proxyBuf, data); err != nil {
			return nil, fmt.Errorf("failed to render proxy: %w", err)
		}
		proxyValue := proxyBuf.String()
		if proxyValue != "" {
			result.Proxy = proxyValue
		}
	}

	// Generate request hash for idempotency
	result.Hash = tr.generateRequestHash(data)

	return result, nil
}

// generateRequestHash generates a unique hash for the request based on data and config
func (tr *TemplateRenderer) generateRequestHash(data map[string]string) string {
	h := sha256.New()
	
	// Include request config in hash
	fmt.Fprintf(h, "method:%s\n", tr.requestConfig.Method)
	fmt.Fprintf(h, "url:%s\n", tr.requestConfig.URL)
	fmt.Fprintf(h, "body:%s\n", tr.requestConfig.Body)
	
	// Include headers
	for key, value := range tr.requestConfig.Headers {
		fmt.Fprintf(h, "header:%s=%s\n", key, value)
	}
	
	// Include row data (sorted by key for consistency)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	// Simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	
	for _, key := range keys {
		fmt.Fprintf(h, "data:%s=%s\n", key, data[key])
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Template functions

// dateFormat formats a date string according to the given layout
func dateFormat(layout, value string) string {
	// Try parsing common formats
	formats := []string{
		"20060102",     // YYYYMMDD
		"2006-01-02",   // YYYY-MM-DD
		"01/02/2006",   // MM/DD/YYYY
		"02/01/2006",   // DD/MM/YYYY
	}
	
	var parsedTime time.Time
	var err error
	
	for _, format := range formats {
		parsedTime, err = time.Parse(format, value)
		if err == nil {
			break
		}
	}
	
	if err != nil {
		return value // Return original if can't parse
	}
	
	return parsedTime.Format(layout)
}

// toE164KR converts Korean phone numbers to E164 format
func toE164KR(phone string) string {
	// Remove all non-digit characters
	cleaned := regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
	
	// Handle Korean phone numbers
	if len(cleaned) == 10 && strings.HasPrefix(cleaned, "0") {
		// 010-XXXX-XXXX format -> +82-10-XXXX-XXXX
		return "+82" + cleaned[1:]
	} else if len(cleaned) == 11 && strings.HasPrefix(cleaned, "01") {
		// 010-XXXX-XXXX format -> +82-10-XXXX-XXXX
		return "+82" + cleaned[1:]
	}
	
	return cleaned
}

// mask masks sensitive data
func mask(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// hash creates a SHA256 hash of the value
func hash(value string) string {
	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", h)
} 