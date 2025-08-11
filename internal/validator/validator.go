package validator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"csvfire/internal/config"
)

// ValidationError represents a validation error
type ValidationError struct {
	Row     int    `json:"row"`
	Column  string `json:"column"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// ValidationResult holds the result of validation
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors"`
	Data   map[string]string `json:"data"` // Processed and normalized data
}

// Validator handles validation and normalization of CSV data
type Validator struct {
	schema *config.Schema
	seen   map[string]map[string]bool // For uniqueness tracking: column -> value -> seen
}

// NewValidator creates a new validator instance
func NewValidator(schema *config.Schema) *Validator {
	seen := make(map[string]map[string]bool)
	
	// Initialize uniqueness tracking maps
	for _, rule := range schema.Uniqueness {
		for _, col := range rule.Columns {
			if seen[col] == nil {
				seen[col] = make(map[string]bool)
			}
		}
	}

	return &Validator{
		schema: schema,
		seen:   seen,
	}
}

// ValidateRow validates a single row of CSV data
func (v *Validator) ValidateRow(rowNum int, data map[string]string) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Errors: make([]ValidationError, 0),
		Data:   make(map[string]string),
	}

	// Process each column according to schema
	for _, colSchema := range v.schema.Columns {
		value, exists := data[colSchema.Name]
		
		// Handle null policy
		if v.schema.NullPolicy.TreatEmptyAsNull && value == "" {
			value = ""
			exists = false
		}

		// Check required fields
		if colSchema.Required && (!exists || value == "") {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Row:     rowNum,
				Column:  colSchema.Name,
				Value:   value,
				Message: "required field is missing or empty",
			})
			continue
		}

		// Skip validation for empty non-required fields
		if !exists || value == "" {
			result.Data[colSchema.Name] = ""
			continue
		}

		// Apply preprocessing
		processedValue := v.preprocess(value, colSchema.Preprocess)

		// Apply normalization
		if colSchema.Normalize != nil {
			if mapped, ok := colSchema.Normalize.Map[processedValue]; ok {
				processedValue = mapped
			}
		}

		// Validate the processed value
		if err := v.validateValue(processedValue, &colSchema); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Row:     rowNum,
				Column:  colSchema.Name,
				Value:   value,
				Message: err.Error(),
			})
			continue
		}

		// Apply transformations
		transformedValue := v.transform(processedValue, colSchema.Transform)

		result.Data[colSchema.Name] = transformedValue
	}

	// Check uniqueness constraints
	if result.Valid {
		v.checkUniqueness(rowNum, result)
	}

	// Validate row-level rules
	if result.Valid {
		v.validateRowRules(rowNum, result)
	}

	return result
}

// preprocess applies preprocessing rules to a value
func (v *Validator) preprocess(value string, rules []config.PreprocessRule) string {
	result := value

	for _, rule := range rules {
		// Apply trim
		if rule.Trim {
			result = strings.TrimSpace(result)
		}

		// Apply removals
		for _, toRemove := range rule.Remove {
			result = strings.ReplaceAll(result, toRemove, "")
		}

		// Apply replacements
		for from, to := range rule.Replace {
			result = strings.ReplaceAll(result, from, to)
		}
	}

	return result
}

// validateValue validates a single value against column schema
func (v *Validator) validateValue(value string, colSchema *config.ColumnSchema) error {
	// Type validation
	if err := v.validateType(value, colSchema.Type, colSchema.Format); err != nil {
		return err
	}

	// Length constraints
	if colSchema.MinLen != nil && len(value) < *colSchema.MinLen {
		return fmt.Errorf("value too short (min %d characters)", *colSchema.MinLen)
	}
	if colSchema.MaxLen != nil && len(value) > *colSchema.MaxLen {
		return fmt.Errorf("value too long (max %d characters)", *colSchema.MaxLen)
	}

	// Regex validation
	if colSchema.Regex != "" {
		matched, err := regexp.MatchString(colSchema.Regex, value)
		if err != nil {
			return fmt.Errorf("regex validation error: %w", err)
		}
		if !matched {
			return fmt.Errorf("value does not match required pattern")
		}
	}

	// Enum validation
	if len(colSchema.Enum) > 0 {
		found := false
		for _, allowed := range colSchema.Enum {
			if value == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value must be one of: %s", strings.Join(colSchema.Enum, ", "))
		}
	}

	// Custom validation rules
	for _, rule := range colSchema.Validators {
		if rule.Regex != "" {
			matched, err := regexp.MatchString(rule.Regex, value)
			if err != nil {
				return fmt.Errorf("custom validation error: %w", err)
			}
			if !matched {
				message := rule.Message
				if message == "" {
					message = "value does not match validation rule"
				}
				return fmt.Errorf(message)
			}
		}
	}

	return nil
}

// validateType validates value against the specified type
func (v *Validator) validateType(value, colType, format string) error {
	switch {
	case colType == "string":
		return nil // No additional validation needed
	case colType == "int":
		_, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
	case colType == "float":
		_, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float: %w", err)
		}
	case strings.HasPrefix(colType, "decimal("):
		_, err := decimal.NewFromString(value)
		if err != nil {
			return fmt.Errorf("invalid decimal: %w", err)
		}
	case strings.HasPrefix(colType, "date"):
		return v.validateDate(value, format)
	default:
		return fmt.Errorf("unsupported column type: %s", colType)
	}
	return nil
}

// validateDate validates date values
func (v *Validator) validateDate(value, format string) error {
	if format == "" {
		format = "20060102" // Default YYYYMMDD
	}

	date, err := time.Parse(format, value)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}

	// Additional validation for Korean birth dates (age 0-120)
	if format == "20060102" {
		now := time.Now()
		age := now.Year() - date.Year()
		if date.After(now.AddDate(-age, 0, 0)) {
			age--
		}
		
		if age < 0 || age > 120 {
			return fmt.Errorf("invalid age: %d (must be 0-120)", age)
		}
	}

	return nil
}

// transform applies transformation rules to a value
func (v *Validator) transform(value string, rules []config.TransformRule) string {
	result := value

	for _, rule := range rules {
		if rule.FormatKoreanPhoneE164 {
			result = formatKoreanPhoneE164(result)
		}
	}

	return result
}

// formatKoreanPhoneE164 formats Korean phone numbers to E164 format
func formatKoreanPhoneE164(phone string) string {
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
	
	return cleaned // Return as-is if not a standard Korean mobile format
}

// checkUniqueness validates uniqueness constraints
func (v *Validator) checkUniqueness(rowNum int, result *ValidationResult) {
	for _, rule := range v.schema.Uniqueness {
		for _, col := range rule.Columns {
			value := result.Data[col]
			if value == "" {
				continue // Skip empty values for uniqueness
			}

			if v.seen[col][value] {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Row:     rowNum,
					Column:  col,
					Value:   value,
					Message: "duplicate value violates uniqueness constraint",
				})
			} else {
				v.seen[col][value] = true
			}
		}
	}
}

// validateRowRules validates row-level rules
func (v *Validator) validateRowRules(rowNum int, result *ValidationResult) {
	for _, rule := range v.schema.RowRules {
		if !v.evaluateRowRule(rule.Expr, result.Data) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Row:     rowNum,
				Column:  "",
				Value:   "",
				Message: fmt.Sprintf("row rule '%s' failed: %s", rule.Name, rule.Expr),
			})
		}
	}
}

// evaluateRowRule evaluates a row rule expression
// This is a simplified implementation - in a production system,
// you might want to use a proper expression evaluator
func (v *Validator) evaluateRowRule(expr string, data map[string]string) bool {
	// Handle age validation for birth dates
	if strings.Contains(expr, "age(birth)") {
		birthStr := data["birth"]
		if birthStr == "" {
			return false
		}

		birth, err := time.Parse("20060102", birthStr)
		if err != nil {
			return false
		}

		now := time.Now()
		age := now.Year() - birth.Year()
		if birth.After(now.AddDate(-age, 0, 0)) {
			age--
		}

		// Replace age(birth) with actual age
		ageExpr := strings.ReplaceAll(expr, "age(birth)", strconv.Itoa(age))
		
		// Simple evaluation for age >= 0 && age <= 120
		if strings.Contains(ageExpr, ">=") && strings.Contains(ageExpr, "&&") && strings.Contains(ageExpr, "<=") {
			parts := strings.Split(ageExpr, "&&")
			if len(parts) == 2 {
				// Check first condition (age >= 0)
				left := strings.TrimSpace(parts[0])
				if strings.Contains(left, ">=") {
					ageParts := strings.Split(left, ">=")
					if len(ageParts) == 2 {
						minAge, err := strconv.Atoi(strings.TrimSpace(ageParts[1]))
						if err != nil || age < minAge {
							return false
						}
					}
				}

				// Check second condition (age <= 120)
				right := strings.TrimSpace(parts[1])
				if strings.Contains(right, "<=") {
					ageParts := strings.Split(right, "<=")
					if len(ageParts) == 2 {
						maxAge, err := strconv.Atoi(strings.TrimSpace(ageParts[1]))
						if err != nil || age > maxAge {
							return false
						}
					}
				}
				return true
			}
		}
	}

	// Default to true for unimplemented expressions
	return true
} 