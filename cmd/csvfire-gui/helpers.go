package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// getRegexPresets returns common regex patterns
func getRegexPresets() []RegexPreset {
	return []RegexPreset{
		{"없음", "", "정규식 검증 없음"},
		{"이메일", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, "이메일 주소 형식"},
		{"휴대폰", `^01[0-9]-[0-9]{4}-[0-9]{4}$`, "휴대폰 번호 (010-1234-5678)"},
		{"휴대폰(숫자만)", `^01[0-9][0-9]{8}$`, "휴대폰 번호 (01012345678)"},
		{"한글이름", `^[가-힣]{2,10}$`, "한글 이름 (2-10자)"},
		{"영문이름", `^[a-zA-Z\s]{2,50}$`, "영문 이름 (2-50자)"},
		{"숫자만", `^[0-9]+$`, "숫자만 허용"},
		{"영문+숫자", `^[a-zA-Z0-9]+$`, "영문자와 숫자만"},
		{"날짜(YYYYMMDD)", `^[0-9]{8}$`, "날짜 형식 (20231201)"},
		{"URL", `^https?://[^\s]+$`, "웹 URL 형식"},
		{"IP주소", `^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`, "IPv4 주소"},
		{"우편번호", `^[0-9]{5}$`, "우편번호 (5자리)"},
	}
}

// formatJSON formats JSON text with proper indentation
func formatJSON(jsonText string) string {
	// 빈 텍스트 처리
	if strings.TrimSpace(jsonText) == "" {
		return jsonText
	}
	
	// 템플릿 변수가 포함된 경우를 위한 전처리
	// {{.variable}} 형태의 템플릿 변수를 임시로 대체
	templateRegex := regexp.MustCompile(`\{\{[^}]+\}\}`)
	templates := templateRegex.FindAllString(jsonText, -1)
	
	// 템플릿 변수를 임시 값으로 대체
	processedText := jsonText
	for i, template := range templates {
		placeholder := fmt.Sprintf("TEMPLATE_VAR_%d", i)
		processedText = strings.Replace(processedText, template, fmt.Sprintf(`"%s"`, placeholder), 1)
	}
	
	// JSON 파싱 시도
	var jsonObj interface{}
	err := json.Unmarshal([]byte(processedText), &jsonObj)
	if err != nil {
		// 파싱 실패시 원본 반환
		return jsonText
	}
	
	// 포맷팅
	formatted, err := json.MarshalIndent(jsonObj, "", "  ")
	if err != nil {
		return jsonText
	}
	
	// 템플릿 변수 복원
	formattedText := string(formatted)
	for i, template := range templates {
		placeholder := fmt.Sprintf(`"TEMPLATE_VAR_%d"`, i)
		formattedText = strings.Replace(formattedText, placeholder, template, 1)
	}
	
	return formattedText
}

// validateJSON checks if the JSON is valid
func validateJSON(jsonText string) error {
	var jsonObj interface{}
	return json.Unmarshal([]byte(jsonText), &jsonObj)
}

// createEnhancedJSONEditor creates a JSON editor with autocomplete and formatting
func (a *App) createEnhancedJSONEditor(placeholder string) *widget.Entry {
	editor := widget.NewMultiLineEntry()
	editor.SetPlaceHolder(placeholder)
	editor.Wrapping = fyne.TextWrapWord
	
	// 키보드 이벤트 처리
	editor.OnChanged = func(text string) {
		// 실시간 JSON 검증 (백그라운드에서)
		go func() {
			if strings.TrimSpace(text) != "" {
				if err := validateJSON(text); err != nil {
					// JSON 오류가 있어도 조용히 처리 (사용자가 입력 중일 수 있음)
				}
			}
		}()
	}
	
	return editor
} 

// generateSchemaYAML generates YAML content from current schema data
func (a *App) generateSchemaYAML() string {
	var yamlContent strings.Builder
	
	yamlContent.WriteString("version: 1\n")
	yamlContent.WriteString("columns:\n")
	
	for _, column := range a.schemaData.Columns {
		yamlContent.WriteString(fmt.Sprintf("  - name: \"%s\"\n", column.Name))
		yamlContent.WriteString(fmt.Sprintf("    type: \"%s\"\n", column.Type))
		yamlContent.WriteString(fmt.Sprintf("    required: %t\n", column.Required))
		
		// Add optional MinLen field
		if column.MinLen > 0 {
			yamlContent.WriteString(fmt.Sprintf("    min_len: %d\n", column.MinLen))
		}
		
		// Add optional MaxLen field
		if column.MaxLen > 0 {
			yamlContent.WriteString(fmt.Sprintf("    max_len: %d\n", column.MaxLen))
		}
		
		// Add optional Enum field
		if len(column.Enum) > 0 {
			yamlContent.WriteString("    enum:\n")
			for _, enumVal := range column.Enum {
				yamlContent.WriteString(fmt.Sprintf("      - \"%s\"\n", enumVal))
			}
		}
		
		// Add validators section if regex is present
		if column.Regex != "" {
			yamlContent.WriteString("    validators:\n")
			yamlContent.WriteString(fmt.Sprintf("      - regex: \"%s\"\n", column.Regex))
		}
		
		yamlContent.WriteString("\n")
	}
	
	// 기본 설정 추가
	yamlContent.WriteString("null_policy:\n")
	yamlContent.WriteString("  treat_empty_as_null: true\n")
	
	return yamlContent.String()
}

// generateRequestYAML generates YAML content from request settings
func (a *App) generateRequestYAML(methodSelect *widget.Select, urlEntry, contentTypeEntry, authEntry, bodyEntry, statusEntry *widget.Entry) string {
	var yamlContent strings.Builder
	
	yamlContent.WriteString(fmt.Sprintf("method: %s\n", methodSelect.Selected))
	yamlContent.WriteString(fmt.Sprintf("url: \"%s\"\n", urlEntry.Text))
	yamlContent.WriteString("headers:\n")
	yamlContent.WriteString(fmt.Sprintf("  Content-Type: \"%s\"\n", contentTypeEntry.Text))
	
	if authEntry.Text != "" {
		yamlContent.WriteString(fmt.Sprintf("  Authorization: \"%s\"\n", authEntry.Text))
	}
	
	yamlContent.WriteString("body: |\n")
	
	// 본문을 인덴트해서 추가
	bodyLines := strings.Split(bodyEntry.Text, "\n")
	for _, line := range bodyLines {
		yamlContent.WriteString(fmt.Sprintf("  %s\n", line))
	}
	
	yamlContent.WriteString("success:\n")
	statusCodes := strings.Split(statusEntry.Text, ",")
	yamlContent.WriteString("  status_in: [")
	for i, code := range statusCodes {
		if i > 0 {
			yamlContent.WriteString(", ")
		}
		yamlContent.WriteString(strings.TrimSpace(code))
	}
	yamlContent.WriteString("]\n")
	
	return yamlContent.String()
}