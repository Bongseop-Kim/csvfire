package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Schema represents the validation schema for CSV data
type Schema struct {
	Version     int                    `yaml:"version"`
	Columns     []ColumnSchema         `yaml:"columns"`
	RowRules    []RowRule              `yaml:"row_rules"`
	Uniqueness  []UniquenessRule       `yaml:"uniqueness"`
	NullPolicy  NullPolicy             `yaml:"null_policy"`
}

// ColumnSchema defines validation rules for a single column
type ColumnSchema struct {
	Name        string              `yaml:"name"`
	Type        string              `yaml:"type"`
	Required    bool                `yaml:"required"`
	Secret      bool                `yaml:"secret"`
	MinLen      *int                `yaml:"min_len,omitempty"`
	MaxLen      *int                `yaml:"max_len,omitempty"`
	Regex       string              `yaml:"regex,omitempty"`
	Enum        []string            `yaml:"enum,omitempty"`
	Range       *RangeRule          `yaml:"range,omitempty"`
	Format      string              `yaml:"format,omitempty"`
	Preprocess  []PreprocessRule    `yaml:"preprocess,omitempty"`
	Validators  []ValidationRule    `yaml:"validators,omitempty"`
	Transform   []TransformRule     `yaml:"transform,omitempty"`
	Normalize   *NormalizeRule      `yaml:"normalize,omitempty"`
}

// RangeRule defines min/max constraints
type RangeRule struct {
	Min *decimal.Decimal `yaml:"min,omitempty"`
	Max *decimal.Decimal `yaml:"max,omitempty"`
}

// PreprocessRule defines preprocessing operations
type PreprocessRule struct {
	Remove   []string          `yaml:"remove,omitempty"`
	Replace  map[string]string `yaml:"replace,omitempty"`
	Trim     bool              `yaml:"trim,omitempty"`
}

// ValidationRule defines custom validation
type ValidationRule struct {
	Regex   string `yaml:"regex,omitempty"`
	Message string `yaml:"message,omitempty"`
}

// TransformRule defines transformation operations
type TransformRule struct {
	FormatKoreanPhoneE164 bool `yaml:"format_korean_phone_e164,omitempty"`
}

// NormalizeRule defines normalization mappings
type NormalizeRule struct {
	Map map[string]string `yaml:"map,omitempty"`
}

// RowRule defines rules that apply to entire rows
type RowRule struct {
	Name string `yaml:"name"`
	Expr string `yaml:"expr"`
}

// UniquenessRule defines uniqueness constraints
type UniquenessRule struct {
	Columns []string `yaml:"columns"`
}

// NullPolicy defines how to handle null/empty values
type NullPolicy struct {
	TreatEmptyAsNull bool `yaml:"treat_empty_as_null"`
}

// LoadSchema loads and parses a schema file
func LoadSchema(filename string) (*Schema, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	var schema Schema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema YAML: %w", err)
	}

	// Validate schema
	if err := validateSchema(&schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	return &schema, nil
}

// validateSchema performs basic validation on the schema structure
func validateSchema(schema *Schema) error {
	if schema.Version != 1 {
		return fmt.Errorf("unsupported schema version: %d", schema.Version)
	}

	if len(schema.Columns) == 0 {
		return fmt.Errorf("no columns defined in schema")
	}

	// Validate column names are unique
	seen := make(map[string]bool)
	for _, col := range schema.Columns {
		if col.Name == "" {
			return fmt.Errorf("column name cannot be empty")
		}
		if seen[col.Name] {
			return fmt.Errorf("duplicate column name: %s", col.Name)
		}
		seen[col.Name] = true

		// Validate column type
		if !isValidColumnType(col.Type) {
			return fmt.Errorf("invalid column type '%s' for column '%s'", col.Type, col.Name)
		}

		// Validate regex if present
		if col.Regex != "" {
			if _, err := regexp.Compile(col.Regex); err != nil {
				return fmt.Errorf("invalid regex for column '%s': %w", col.Name, err)
			}
		}

		// Validate validation rules
		for _, rule := range col.Validators {
			if rule.Regex != "" {
				if _, err := regexp.Compile(rule.Regex); err != nil {
					return fmt.Errorf("invalid regex in validation rule for column '%s': %w", col.Name, err)
				}
			}
		}
	}

	return nil
}

// isValidColumnType checks if the given column type is supported
func isValidColumnType(colType string) bool {
	switch {
	case colType == "string":
		return true
	case colType == "int":
		return true
	case colType == "float":
		return true
	case strings.HasPrefix(colType, "decimal("):
		return isValidDecimalType(colType)
	case strings.HasPrefix(colType, "date"):
		return true
	default:
		return false
	}
}

// isValidDecimalType validates decimal type format: decimal(precision,scale)
func isValidDecimalType(colType string) bool {
	if !strings.HasPrefix(colType, "decimal(") || !strings.HasSuffix(colType, ")") {
		return false
	}

	params := strings.TrimPrefix(strings.TrimSuffix(colType, ")"), "decimal(")
	parts := strings.Split(params, ",")
	if len(parts) != 2 {
		return false
	}

	precision, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	scale, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

	return err1 == nil && err2 == nil && precision > 0 && scale >= 0 && scale <= precision
}

// GetColumnByName returns the column schema for the given name
func (s *Schema) GetColumnByName(name string) *ColumnSchema {
	for _, col := range s.Columns {
		if col.Name == name {
			return &col
		}
	}
	return nil
}

// GetColumnNames returns all column names in order
func (s *Schema) GetColumnNames() []string {
	names := make([]string, len(s.Columns))
	for i, col := range s.Columns {
		names[i] = col.Name
	}
	return names
} 