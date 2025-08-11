package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"csvfire/internal/config"
	"csvfire/internal/request"
	"csvfire/internal/validator"
)

// Runner handles concurrent execution of HTTP requests
type Runner struct {
	schema        *config.Schema
	requestConfig *config.RequestConfig
	validator     *validator.Validator
	renderer      *request.TemplateRenderer
	client        *request.Client
	limiter       *rate.Limiter
	concurrency   int
	checkpoints   map[string]bool // For resume functionality
	checkpointMu  sync.RWMutex
}

// RunConfig holds configuration for running requests
type RunConfig struct {
	Concurrency int
	RateLimit   float64 // requests per second
	Timeout     time.Duration
	Resume      bool
}

// RowTask represents a single row to be processed
type RowTask struct {
	RowNumber int
	Data      map[string]string
	RequestID string
}

// RunResult holds the results of processing
type RunResult struct {
	TotalRows     int
	SuccessRows   int
	FailedRows    int
	SkippedRows   int
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
}

// ResultCallback is called for each processed row
type ResultCallback func(rowNum int, validationResult *validator.ValidationResult, requestResult *request.RequestResult)

// NewRunner creates a new runner instance
func NewRunner(schema *config.Schema, requestConfig *config.RequestConfig, runConfig *RunConfig) (*Runner, error) {
	// Create validator
	val := validator.NewValidator(schema)

	// Create template renderer
	renderer, err := request.NewTemplateRenderer(requestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create template renderer: %w", err)
	}

	// Create HTTP client
	client := request.NewClient(requestConfig, runConfig.Timeout)

	// Create rate limiter
	var limiter *rate.Limiter
	if runConfig.RateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(runConfig.RateLimit), 1)
	}

	return &Runner{
		schema:        schema,
		requestConfig: requestConfig,
		validator:     val,
		renderer:      renderer,
		client:        client,
		limiter:       limiter,
		concurrency:   runConfig.Concurrency,
		checkpoints:   make(map[string]bool),
	}, nil
}

// LoadCheckpoints loads checkpoint data for resume functionality
func (r *Runner) LoadCheckpoints(checkpoints map[string]bool) {
	r.checkpointMu.Lock()
	defer r.checkpointMu.Unlock()
	
	for hash := range checkpoints {
		r.checkpoints[hash] = true
	}
}

// Run processes rows concurrently
func (r *Runner) Run(ctx context.Context, rows <-chan RowTask, callback ResultCallback) *RunResult {
	result := &RunResult{
		StartTime: time.Now(),
	}

	// Create worker pool
	taskChan := make(chan RowTask, r.concurrency*2) // Buffer to prevent blocking
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < r.concurrency; i++ {
		wg.Add(1)
		go r.worker(ctx, taskChan, callback, result, &wg)
	}

	// Feed tasks to workers
	go func() {
		defer close(taskChan)
		for task := range rows {
			select {
			case taskChan <- task:
				result.TotalRows++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for all workers to complete
	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result
}

// worker processes individual tasks
func (r *Runner) worker(ctx context.Context, tasks <-chan RowTask, callback ResultCallback, result *RunResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
			r.processTask(ctx, task, callback, result)
		}
	}
}

// processTask processes a single task
func (r *Runner) processTask(ctx context.Context, task RowTask, callback ResultCallback, result *RunResult) {
	// Rate limiting
	if r.limiter != nil {
		if err := r.limiter.Wait(ctx); err != nil {
			return // Context cancelled
		}
	}

	// Validate the row
	validationResult := r.validator.ValidateRow(task.RowNumber, task.Data)
	
	var requestResult *request.RequestResult

	if validationResult.Valid {
		// Generate request hash for idempotency
		requestHash := r.generateRequestHash(validationResult.Data)
		
		// Check if this request was already processed (resume functionality)
		if r.isAlreadyProcessed(requestHash) {
			result.SkippedRows++
			return
		}

		// Render request template
		requestData, err := r.renderer.Render(validationResult.Data)
		if err != nil {
			// Create a dummy request result for template errors
			requestResult = &request.RequestResult{
				RequestID:     task.RequestID,
				Success:       false,
				ErrorCategory: "template_error",
				ErrorDetail:   err.Error(),
			}
		} else {
			// Execute HTTP request
			requestData.Hash = requestHash
			requestResult = r.client.Execute(ctx, requestData, task.RequestID)
			
			// Mark as processed if successful
			if requestResult.Success {
				r.markAsProcessed(requestHash)
				result.SuccessRows++
			} else {
				result.FailedRows++
			}
		}
	} else {
		// Validation failed
		result.FailedRows++
		requestResult = &request.RequestResult{
			RequestID:     task.RequestID,
			Success:       false,
			ErrorCategory: "validation_error",
			ErrorDetail:   "Row validation failed",
		}
	}

	// Call callback with results
	if callback != nil {
		callback(task.RowNumber, validationResult, requestResult)
	}
}

// generateRequestHash generates a hash for the request data
func (r *Runner) generateRequestHash(data map[string]string) string {
	h := sha256.New()
	
	// Include request config in hash for consistency
	fmt.Fprintf(h, "method:%s\n", r.requestConfig.Method)
	fmt.Fprintf(h, "url:%s\n", r.requestConfig.URL)
	fmt.Fprintf(h, "body:%s\n", r.requestConfig.Body)
	
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

// isAlreadyProcessed checks if a request hash has been processed
func (r *Runner) isAlreadyProcessed(hash string) bool {
	r.checkpointMu.RLock()
	defer r.checkpointMu.RUnlock()
	
	return r.checkpoints[hash]
}

// markAsProcessed marks a request hash as processed
func (r *Runner) markAsProcessed(hash string) {
	r.checkpointMu.Lock()
	defer r.checkpointMu.Unlock()
	
	r.checkpoints[hash] = true
}

// GetProcessedHashes returns all processed request hashes
func (r *Runner) GetProcessedHashes() map[string]bool {
	r.checkpointMu.RLock()
	defer r.checkpointMu.RUnlock()
	
	result := make(map[string]bool)
	for hash := range r.checkpoints {
		result[hash] = true
	}
	
	return result
} 