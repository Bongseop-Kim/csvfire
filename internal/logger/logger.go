package logger

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csvfire/internal/config"
	"csvfire/internal/request"
	"csvfire/internal/validator"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp       time.Time                     `json:"timestamp"`
	Row             int                           `json:"row"`
	RequestID       string                        `json:"request_id"`
	StatusCode      int                           `json:"status_code"`
	Success         bool                          `json:"success"`
	LatencyMs       int64                         `json:"latency_ms"`
	Retries         int                           `json:"retries"`
	ErrorCategory   string                        `json:"error_category"`
	ErrorDetail     string                        `json:"error_detail"`
	ResponsePreview string                        `json:"response_preview"`
	RequestHash     string                        `json:"request_hash"`
}

// ValidationLogEntry represents a validation error log entry
type ValidationLogEntry struct {
	Timestamp time.Time                `json:"timestamp"`
	Row       int                      `json:"row"`
	Errors    []validator.ValidationError `json:"errors"`
}

// Logger handles CSV logging with channels for concurrent writing
type Logger struct {
	schema          *config.Schema
	logDir          string
	sentLogFile     *os.File
	sentLogWriter   *csv.Writer
	errorLogFile    *os.File
	errorLogWriter  *csv.Writer
	validateLogFile *os.File
	validateLogWriter *csv.Writer
	logChan         chan LogEntry
	validateLogChan chan ValidationLogEntry
	failedRows      []FailedRow
	stopChan        chan struct{}
	doneChan        chan struct{}
}

// FailedRow represents a failed row for export
type FailedRow struct {
	RowNumber int
	Data      map[string]string
	Reason    string
}

// NewLogger creates a new logger instance
func NewLogger(schema *config.Schema, logDir string) (*Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger := &Logger{
		schema:          schema,
		logDir:          logDir,
		logChan:         make(chan LogEntry, 1000),
		validateLogChan: make(chan ValidationLogEntry, 1000),
		failedRows:      make([]FailedRow, 0),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}

	// Initialize log files
	if err := logger.initLogFiles(); err != nil {
		return nil, err
	}

	// Start background logger
	go logger.runLogger()

	return logger, nil
}

// initLogFiles initializes CSV log files
func (l *Logger) initLogFiles() error {
	// Initialize sent.csv
	sentLogPath := filepath.Join(l.logDir, "sent.csv")
	var err error
	l.sentLogFile, err = os.Create(sentLogPath)
	if err != nil {
		return fmt.Errorf("failed to create sent log file: %w", err)
	}
	l.sentLogWriter = csv.NewWriter(l.sentLogFile)

	// Write header for sent.csv
	sentHeaders := []string{
		"ts", "row", "request_id", "status_code", "success", "latency_ms",
		"retries", "error_category", "error_detail", "response_preview", "request_hash",
	}
	if err := l.sentLogWriter.Write(sentHeaders); err != nil {
		return fmt.Errorf("failed to write sent log header: %w", err)
	}
	l.sentLogWriter.Flush()

	// Initialize request_errors.csv
	errorLogPath := filepath.Join(l.logDir, "request_errors.csv")
	l.errorLogFile, err = os.Create(errorLogPath)
	if err != nil {
		return fmt.Errorf("failed to create error log file: %w", err)
	}
	l.errorLogWriter = csv.NewWriter(l.errorLogFile)

	// Write header for request_errors.csv
	errorHeaders := []string{
		"ts", "row", "request_id", "error_category", "error_detail", "status_code",
	}
	if err := l.errorLogWriter.Write(errorHeaders); err != nil {
		return fmt.Errorf("failed to write error log header: %w", err)
	}
	l.errorLogWriter.Flush()

	// Initialize validate_errors.csv
	validateLogPath := filepath.Join(l.logDir, "validate_errors.csv")
	l.validateLogFile, err = os.Create(validateLogPath)
	if err != nil {
		return fmt.Errorf("failed to create validation log file: %w", err)
	}
	l.validateLogWriter = csv.NewWriter(l.validateLogFile)

	// Write header for validate_errors.csv
	validateHeaders := []string{
		"ts", "row", "column", "value", "message",
	}
	if err := l.validateLogWriter.Write(validateHeaders); err != nil {
		return fmt.Errorf("failed to write validation log header: %w", err)
	}
	l.validateLogWriter.Flush()

	return nil
}

// LogRequest logs a request result
func (l *Logger) LogRequest(rowNum int, validationResult *validator.ValidationResult, requestResult *request.RequestResult) {
	// Log validation errors
	if !validationResult.Valid {
		l.validateLogChan <- ValidationLogEntry{
			Timestamp: time.Now(),
			Row:       rowNum,
			Errors:    validationResult.Errors,
		}

		// Add to failed rows
		l.addFailedRow(rowNum, validationResult.Data, "validation_failed")
	}

	// Log request result
	if requestResult != nil {
		entry := LogEntry{
			Timestamp:       time.Now(),
			Row:             rowNum,
			RequestID:       requestResult.RequestID,
			StatusCode:      requestResult.StatusCode,
			Success:         requestResult.Success,
			LatencyMs:       requestResult.LatencyMs,
			Retries:         requestResult.Retries,
			ErrorCategory:   requestResult.ErrorCategory,
			ErrorDetail:     requestResult.ErrorDetail,
			ResponsePreview: requestResult.ResponsePreview,
		}

		l.logChan <- entry

		// Add to failed rows if request failed
		if !requestResult.Success {
			reason := requestResult.ErrorCategory
			if reason == "" {
				reason = "request_failed"
			}
			l.addFailedRow(rowNum, validationResult.Data, reason)
		}
	}
}

// addFailedRow adds a row to the failed rows list
func (l *Logger) addFailedRow(rowNum int, data map[string]string, reason string) {
	l.failedRows = append(l.failedRows, FailedRow{
		RowNumber: rowNum,
		Data:      data,
		Reason:    reason,
	})
}

// runLogger runs the background logging goroutine
func (l *Logger) runLogger() {
	defer close(l.doneChan)

	for {
		select {
		case entry := <-l.logChan:
			l.writeSentLog(entry)
			if !entry.Success {
				l.writeErrorLog(entry)
			}

		case validateEntry := <-l.validateLogChan:
			l.writeValidationLog(validateEntry)

		case <-l.stopChan:
			// Drain remaining logs
			for {
				select {
				case entry := <-l.logChan:
					l.writeSentLog(entry)
					if !entry.Success {
						l.writeErrorLog(entry)
					}
				case validateEntry := <-l.validateLogChan:
					l.writeValidationLog(validateEntry)
				default:
					return
				}
			}
		}
	}
}

// writeSentLog writes to sent.csv
func (l *Logger) writeSentLog(entry LogEntry) {
	record := []string{
		entry.Timestamp.Format(time.RFC3339),
		fmt.Sprintf("%d", entry.Row),
		entry.RequestID,
		fmt.Sprintf("%d", entry.StatusCode),
		fmt.Sprintf("%t", entry.Success),
		fmt.Sprintf("%d", entry.LatencyMs),
		fmt.Sprintf("%d", entry.Retries),
		entry.ErrorCategory,
		l.maskSensitiveData(entry.ErrorDetail),
		l.maskSensitiveData(entry.ResponsePreview),
		entry.RequestHash,
	}

	if err := l.sentLogWriter.Write(record); err != nil {
		fmt.Printf("Error writing to sent log: %v\n", err)
	}
	l.sentLogWriter.Flush()
}

// writeErrorLog writes to request_errors.csv
func (l *Logger) writeErrorLog(entry LogEntry) {
	record := []string{
		entry.Timestamp.Format(time.RFC3339),
		fmt.Sprintf("%d", entry.Row),
		entry.RequestID,
		entry.ErrorCategory,
		l.maskSensitiveData(entry.ErrorDetail),
		fmt.Sprintf("%d", entry.StatusCode),
	}

	if err := l.errorLogWriter.Write(record); err != nil {
		fmt.Printf("Error writing to error log: %v\n", err)
	}
	l.errorLogWriter.Flush()
}

// writeValidationLog writes to validate_errors.csv
func (l *Logger) writeValidationLog(entry ValidationLogEntry) {
	for _, validationError := range entry.Errors {
		record := []string{
			entry.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", entry.Row),
			validationError.Column,
			l.maskSensitiveData(validationError.Value),
			validationError.Message,
		}

		if err := l.validateLogWriter.Write(record); err != nil {
			fmt.Printf("Error writing to validation log: %v\n", err)
		}
	}
	l.validateLogWriter.Flush()
}

// maskSensitiveData masks sensitive information in log data
func (l *Logger) maskSensitiveData(value string) string {
	// Check if any column in the schema is marked as secret
	for _, col := range l.schema.Columns {
		if col.Secret && strings.Contains(value, col.Name) {
			// Simple masking - replace with asterisks
			if len(value) <= 4 {
				return strings.Repeat("*", len(value))
			}
			return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
		}
	}
	return value
}

// Close closes the logger and all its files
func (l *Logger) Close() {
	close(l.stopChan)
	<-l.doneChan

	if l.sentLogWriter != nil {
		l.sentLogWriter.Flush()
	}
	if l.sentLogFile != nil {
		l.sentLogFile.Close()
	}

	if l.errorLogWriter != nil {
		l.errorLogWriter.Flush()
	}
	if l.errorLogFile != nil {
		l.errorLogFile.Close()
	}

	if l.validateLogWriter != nil {
		l.validateLogWriter.Flush()
	}
	if l.validateLogFile != nil {
		l.validateLogFile.Close()
	}
}

// ExportFailedRows exports failed rows to a CSV file
func (l *Logger) ExportFailedRows(filename string) error {
	if len(l.failedRows) == 0 {
		return nil // No failed rows to export
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create failed rows file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header (original column names plus reason)
	headers := l.schema.GetColumnNames()
	headers = append(headers, "failure_reason")
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write failed rows
	for _, failedRow := range l.failedRows {
		record := make([]string, len(headers))
		
		// Fill original column data
		for i, colName := range l.schema.GetColumnNames() {
			if value, exists := failedRow.Data[colName]; exists {
				record[i] = value
			}
		}
		
		// Add failure reason
		record[len(record)-1] = failedRow.Reason

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write failed row: %w", err)
		}
	}

	return nil
}

// GetFailedRowCount returns the number of failed rows
func (l *Logger) GetFailedRowCount() int {
	return len(l.failedRows)
} 