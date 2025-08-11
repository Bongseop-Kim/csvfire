package reader

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"csvfire/internal/config"
	"csvfire/internal/runner"
)

// CSVReader handles streaming CSV reading
type CSVReader struct {
	schema   *config.Schema
	filename string
}

// NewCSVReader creates a new CSV reader
func NewCSVReader(schema *config.Schema, filename string) *CSVReader {
	return &CSVReader{
		schema:   schema,
		filename: filename,
	}
}

// ReadRows reads CSV rows and sends them to the tasks channel
func (r *CSVReader) ReadRows(tasksChan chan<- runner.RowTask) error {
	defer close(tasksChan)

	file, err := os.Open(r.filename)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	// Create CSV reader with buffering for better performance
	bufferedReader := bufio.NewReader(file)
	csvReader := csv.NewReader(bufferedReader)
	
	// Configure CSV reader
	csvReader.FieldsPerRecord = len(r.schema.Columns)
	csvReader.TrimLeadingSpace = true

	// Read header row
	headers, err := csvReader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Validate headers match schema
	expectedHeaders := r.schema.GetColumnNames()
	if err := r.validateHeaders(headers, expectedHeaders); err != nil {
		return fmt.Errorf("header validation failed: %w", err)
	}

	// Read data rows
	rowNumber := 1 // Start from 1 (excluding header)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read CSV row %d: %w", rowNumber, err)
		}

		// Convert record to map
		data := make(map[string]string)
		for i, value := range record {
			if i < len(headers) {
				data[headers[i]] = value
			}
		}

		// Generate request ID
		requestID := fmt.Sprintf("req_%d_%d", rowNumber, r.generateRowHash(data))

		// Send task
		task := runner.RowTask{
			RowNumber: rowNumber,
			Data:      data,
			RequestID: requestID,
		}

		tasksChan <- task
		rowNumber++
	}

	return nil
}

// validateHeaders validates that CSV headers match schema columns
func (r *CSVReader) validateHeaders(headers, expectedHeaders []string) error {
	if len(headers) != len(expectedHeaders) {
		return fmt.Errorf("header count mismatch: got %d, expected %d", len(headers), len(expectedHeaders))
	}

	for i, header := range headers {
		if header != expectedHeaders[i] {
			return fmt.Errorf("header mismatch at position %d: got '%s', expected '%s'", i, header, expectedHeaders[i])
		}
	}

	return nil
}

// generateRowHash generates a simple hash for the row data
func (r *CSVReader) generateRowHash(data map[string]string) int {
	hash := 0
	for key, value := range data {
		for _, c := range key + value {
			hash = 31*hash + int(c)
		}
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// CountRows counts the number of data rows in the CSV file (excluding header)
func (r *CSVReader) CountRows() (int, error) {
	file, err := os.Open(r.filename)
	if err != nil {
		return 0, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	bufferedReader := bufio.NewReader(file)
	csvReader := csv.NewReader(bufferedReader)
	csvReader.FieldsPerRecord = -1 // Allow variable field count for counting

	count := 0
	isFirstRow := true

	for {
		_, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read CSV for counting: %w", err)
		}

		if isFirstRow {
			isFirstRow = false
			continue // Skip header row
		}

		count++
	}

	return count, nil
}

// GetPreviewRows returns the first N rows for preview
func (r *CSVReader) GetPreviewRows(limit int) ([]map[string]string, error) {
	file, err := os.Open(r.filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	bufferedReader := bufio.NewReader(file)
	csvReader := csv.NewReader(bufferedReader)
	csvReader.FieldsPerRecord = len(r.schema.Columns)
	csvReader.TrimLeadingSpace = true

	// Read header row
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	var rows []map[string]string
	count := 0

	for count < limit {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		// Convert record to map
		data := make(map[string]string)
		for i, value := range record {
			if i < len(headers) {
				data[headers[i]] = value
			}
		}

		rows = append(rows, data)
		count++
	}

	return rows, nil
} 