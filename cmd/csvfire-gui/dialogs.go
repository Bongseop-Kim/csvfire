package main

import (
	"fmt"
	"strings"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// showTemplateHelp shows a help dialog for template variables
func (a *App) showTemplateHelp() {
	helpWindow := a.fyneApp.NewWindow("템플릿 변수 도움말")
	helpWindow.Resize(fyne.NewSize(600, 500))
	
	helpText := `📝 요청 템플릿에서 사용 가능한 변수들:

🔹 기본 CSV 컬럼 변수:
   {{.column_name}} - CSV의 각 컬럼 값 (예: {{.name}}, {{.email}})

🔹 내장 템플릿 함수:
   {{ dateFormat "2006-01-02" .birth }}     - 날짜 포맷 변경
   {{ toE164KR .phone }}                    - 한국 전화번호를 E164 형식으로
   {{ mask .password }}                     - 민감한 데이터 마스킹
   {{ hash .email }}                        - 문자열 해시값 생성
   {{ now }}                                - 현재 시간

🔹 조건부 표현:
   {{ if .token }}Bearer {{.token}}{{ end }} - 값이 있을 때만 출력

🔹 예시 JSON:
{
  "name": "{{.name}}",
  "email": "{{.email}}",
  "phone": "{{ toE164KR .phone }}",
  "created_at": "{{ dateFormat "2006-01-02T15:04:05Z" now }}",
  "auth": "{{ if .token }}Bearer {{.token}}{{ else }}default-token{{ end }}"
}

💡 CSV 컬럼명이 user_name이면 {{.user_name}}으로 사용하세요!`

	helpLabel := widget.NewLabel(helpText)
	helpLabel.Wrapping = fyne.TextWrapWord
	
	closeBtn := widget.NewButton("닫기", func() {
		helpWindow.Close()
	})
	
	content := container.NewBorder(
		widget.NewLabel("📚 템플릿 변수 사용법"),
		closeBtn,
		nil, nil,
		container.NewScroll(helpLabel),
	)
	
	helpWindow.SetContent(content)
	helpWindow.Show()
}

// showSchemaConfigDialog shows a dialog for configuring schema settings
func (a *App) showSchemaConfigDialog() {
	// 스키마 설정을 위한 다이얼로그 창
	schemaWindow := a.fyneApp.NewWindow("스키마 설정")
	schemaWindow.Resize(fyne.NewSize(700, 600))
	
	// 컬럼 컨테이너 (동적으로 업데이트)
	columnContainer := container.NewVBox()
	
	// 컬럼 리스트 업데이트 함수
	var updateColumnList func()
	updateColumnList = func() {
		columnContainer.Objects = nil // 기존 내용 제거
		
		for i, column := range a.schemaData.Columns {
			index := i // 클로저를 위한 인덱스 복사
			
			// 컬럼명 입력 (너비 확대)
			nameEntry := widget.NewEntry()
			nameEntry.SetText(column.Name)
			nameEntry.SetPlaceHolder("컬럼명 (예: user_name, email, phone)")
			
			// 타입 선택
			typeSelect := widget.NewSelect([]string{"string", "int", "float", "decimal", "date"}, nil)
			typeSelect.SetSelected(column.Type)
			
			// 필수 여부
			requiredCheck := widget.NewCheck("필수", nil)
			requiredCheck.SetChecked(column.Required)
			
			// 삭제 버튼
			deleteBtn := widget.NewButton("🗑️", func() {
				a.removeColumn(index)
				updateColumnList() // 리스트 업데이트
			})
			
			// 정규식 프리셋 선택
			regexPresets := getRegexPresets()
			regexOptions := make([]string, len(regexPresets))
			for j, preset := range regexPresets {
				regexOptions[j] = preset.Name
			}
			
			regexSelect := widget.NewSelect(regexOptions, nil)
			
			// 현재 정규식과 매칭되는 프리셋 찾기
			selectedPreset := "없음"
			for _, preset := range regexPresets {
				if preset.Pattern == column.Regex {
					selectedPreset = preset.Name
					break
				}
			}
			regexSelect.SetSelected(selectedPreset)
			
			// 정규식 설명 라벨
			regexDescLabel := widget.NewLabel("")
			
			// 정규식 선택 시 설명 업데이트
			regexSelect.OnChanged = func(selected string) {
				for _, preset := range regexPresets {
					if preset.Name == selected {
						if index < len(a.schemaData.Columns) {
							a.schemaData.Columns[index].Regex = preset.Pattern
							regexDescLabel.SetText(preset.Description)
						}
						break
					}
				}
			}
			
			// 초기 설명 설정
			for _, preset := range regexPresets {
				if preset.Name == selectedPreset {
					regexDescLabel.SetText(preset.Description)
					break
				}
			}
			
			// 이벤트 핸들러 설정
			nameEntry.OnChanged = func(text string) {
				if index < len(a.schemaData.Columns) {
					a.schemaData.Columns[index].Name = text
				}
			}
			typeSelect.OnChanged = func(value string) {
				if index < len(a.schemaData.Columns) {
					a.schemaData.Columns[index].Type = value
				}
			}
			requiredCheck.OnChanged = func(checked bool) {
				if index < len(a.schemaData.Columns) {
					a.schemaData.Columns[index].Required = checked
				}
			}
			// 컬럼 UI 생성 - 더 넓은 레이아웃
			columnUI := container.NewVBox(
				widget.NewCard(fmt.Sprintf("컬럼 %d", index+1), "",
					container.NewVBox(
						// 첫 번째 행: 컬럼명 (넓게)
						container.NewBorder(nil, nil, 
							widget.NewLabel("이름:"), 
							nil, 
							nameEntry),
						
						// 두 번째 행: 타입, 필수, 삭제
						container.NewHBox(
							widget.NewLabel("타입:"),
							typeSelect,
							layout.NewSpacer(),
							requiredCheck,
							deleteBtn,
						),
						
						// 세 번째 행: 정규식 선택
						container.NewBorder(nil, nil, 
							widget.NewLabel("검증:"), 
							nil, 
							regexSelect),
						
						// 네 번째 행: 정규식 설명
						container.NewHBox(
							widget.NewLabel("📝"),
							regexDescLabel,
						),
					),
				),
			)
			
			columnContainer.Add(columnUI)
		}
		
		columnContainer.Refresh()
	}
	
	// 초기 컬럼 리스트 생성
	updateColumnList()
	
	// 버튼들
	addColumnBtn := widget.NewButton("➕ 컬럼 추가", func() {
		a.addColumn()
		updateColumnList()
		a.logMessage("새 컬럼이 추가되었습니다")
	})
	
	saveSchemaBtn := widget.NewButton("💾 스키마 저장", func() {
		a.saveSchemaToFile()
		schemaWindow.Close()
	})
	
	cancelBtn := widget.NewButton("❌ 취소", func() {
		schemaWindow.Close()
	})
	
	content := container.NewBorder(
		widget.NewLabel("📋 스키마 컬럼 설정"),
		container.NewHBox(addColumnBtn, layout.NewSpacer(), cancelBtn, saveSchemaBtn),
		nil, nil,
		container.NewScroll(columnContainer),
	)
	
	schemaWindow.SetContent(content)
	schemaWindow.Show()
}

// showRequestConfigDialog shows a dialog for configuring request settings
func (a *App) showRequestConfigDialog() {
	// 요청 설정을 위한 다이얼로그 창 (크기 대폭 확대)
	requestWindow := a.fyneApp.NewWindow("HTTP 요청 설정")
	requestWindow.Resize(fyne.NewSize(800, 700))
	
	// 요청 설정 필드들
	methodSelect := widget.NewSelect([]string{"GET", "POST", "PUT", "DELETE", "PATCH"}, nil)
	methodSelect.SetSelected("POST")
	
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://api.example.com/users")
	
	contentTypeEntry := widget.NewEntry()
	contentTypeEntry.SetText("application/json")
	
	authEntry := widget.NewEntry()
	authEntry.SetPlaceHolder("Bearer {{.token}} 또는 고정 값")
	
	// 요청 본문 에디터 (대폭 확대!)
	bodyEntry := a.createEnhancedJSONEditor(`{
  "name": "{{.name}}",
  "email": "{{.email}}",
  "phone": "{{.phone}}",
  "birth": "{{.birth}}",
  "gender": "{{.gender}}",
  "timestamp": "{{ dateFormat "2006-01-02T15:04:05Z" .birth }}",
  "metadata": {
    "source": "csvfire",
    "processed_at": "{{ dateFormat "2006-01-02T15:04:05Z" now }}"
  }
}`)

	// JSON 포맷팅 버튼
	formatBtn := widget.NewButton("🔧 포맷팅", func() {
		currentText := bodyEntry.Text
		if strings.TrimSpace(currentText) == "" {
			dialog.ShowInformation("포맷팅", "포맷팅할 JSON 내용이 없습니다.", requestWindow)
			return
		}
		
		formatted := formatJSON(currentText)
		if formatted != currentText {
			bodyEntry.SetText(formatted)
			dialog.ShowInformation("포맷팅 완료", "JSON이 성공적으로 포맷팅되었습니다!", requestWindow)
		} else {
			dialog.ShowInformation("포맷팅", "이미 올바른 형식이거나 유효하지 않은 JSON입니다.", requestWindow)
		}
	})
	
	// JSON 검증 버튼
	validateBtn := widget.NewButton("✅ 검증", func() {
		currentText := bodyEntry.Text
		if strings.TrimSpace(currentText) == "" {
			dialog.ShowInformation("검증", "검증할 JSON 내용이 없습니다.", requestWindow)
			return
		}
		
		if err := validateJSON(currentText); err != nil {
			dialog.ShowError(fmt.Errorf("JSON 문법 오류: %v", err), requestWindow)
		} else {
			dialog.ShowInformation("검증 완료", "✅ 유효한 JSON입니다!", requestWindow)
		}
	})
	
	// 템플릿 변수 자동 삽입 버튼들
	insertNameBtn := widget.NewButton("{{.name}}", func() {
		bodyEntry.SetText(bodyEntry.Text + "{{.name}}")
	})
	insertEmailBtn := widget.NewButton("{{.email}}", func() {
		bodyEntry.SetText(bodyEntry.Text + "{{.email}}")
	})
	insertPhoneBtn := widget.NewButton("{{.phone}}", func() {
		bodyEntry.SetText(bodyEntry.Text + "{{.phone}}")
	})
	insertDateBtn := widget.NewButton("dateFormat", func() {
		bodyEntry.SetText(bodyEntry.Text + `{{ dateFormat "2006-01-02" .birth }}`)
	})
	
	// 성공 조건
	statusEntry := widget.NewEntry()
	statusEntry.SetText("200,201")
	
	// 버튼들
	saveRequestBtn := widget.NewButton("💾 요청 설정 저장", func() {
		a.saveRequestToFile(methodSelect, urlEntry, contentTypeEntry, authEntry, bodyEntry, statusEntry)
		requestWindow.Close()
	})
	
	cancelBtn := widget.NewButton("❌ 취소", func() {
		requestWindow.Close()
	})
	
	// 템플릿 도움말 버튼
	helpBtn := widget.NewButton("❓ 템플릿 도움말", func() {
		a.showTemplateHelp()
	})
	
	// 상단: 기본 설정 (컴팩트하게)
	basicSettings := widget.NewCard("🔧 기본 설정", "",
		container.NewVBox(
			// 첫 번째 행: 메소드와 URL (한 줄로 전체 너비)
			container.NewBorder(nil, nil, 
				container.NewHBox(widget.NewLabel("메소드:"), methodSelect), 
				nil, 
				container.NewBorder(nil, nil, widget.NewLabel("URL:"), nil, urlEntry)),
			
			// 두 번째 행: Content-Type
			container.NewBorder(nil, nil, 
				widget.NewLabel("Content-Type:"), 
				nil, 
				contentTypeEntry),
			
			// 세 번째 행: Authorization
			container.NewBorder(nil, nil, 
				widget.NewLabel("Authorization:"), 
				nil, 
				authEntry),
		),
	)
	
	// 중앙: 요청 본문 (메인 영역!)
	bodyCard := widget.NewCard("📝 요청 본문 (JSON Template)", "",
		container.NewBorder(
			// 상단: 도구 모음
			container.NewVBox(
				container.NewHBox(
					widget.NewLabel("💡 사용 가능한 변수: {{.name}}, {{.email}}, {{.phone}} 등"),
					layout.NewSpacer(),
					helpBtn,
				),
				// JSON 편집 도구들
				container.NewHBox(
					widget.NewLabel("🛠️ 편집 도구:"),
					formatBtn,
					validateBtn,
					widget.NewSeparator(),
					widget.NewLabel("📝 자동 삽입:"),
					insertNameBtn,
					insertEmailBtn,
					insertPhoneBtn,
					insertDateBtn,
				),
			),
			nil, nil, nil,
			bodyEntry, // 스크롤 제거하고 직접 배치
		),
	)
	
	// 하단: 성공 조건 (컴팩트하게)
	successSettings := widget.NewCard("✅ 성공 조건", "",
		container.NewBorder(nil, nil, 
			widget.NewLabel("상태 코드:"), 
			nil, 
			statusEntry),
	)
	
	// 메인 레이아웃: 상하 분할로 본문 영역 최대화
	content := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("🌐 HTTP 요청 설정"),
			basicSettings,
		), // 상단
		container.NewVBox(
			successSettings,
			container.NewHBox(layout.NewSpacer(), cancelBtn, saveRequestBtn),
		), // 하단
		nil, nil, // 좌우
		bodyCard, // 중앙 (메인 본문 영역)
	)
	
	requestWindow.SetContent(content)
	requestWindow.Show()
}

// saveSchemaToFile saves the current schema to a YAML file
func (a *App) saveSchemaToFile() {
	if len(a.schemaData.Columns) == 0 {
		a.logMessage("❌ 저장할 컬럼이 없습니다")
		return
	}
	
	// 파일 저장 다이얼로그
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			a.logMessage(fmt.Sprintf("❌ 파일 저장 오류: %v", err))
			return
		}
		if writer == nil {
			return // 사용자가 취소
		}
		defer writer.Close()
		
		// YAML 형식으로 스키마 생성
		yamlContent := a.generateSchemaYAML()
		
		// 파일에 쓰기
		_, err = writer.Write([]byte(yamlContent))
		if err != nil {
			a.logMessage(fmt.Sprintf("❌ 파일 쓰기 오류: %v", err))
			return
		}
		
		// 성공 메시지
		filename := writer.URI().Name()
		a.logMessage(fmt.Sprintf("✅ 스키마가 저장되었습니다: %s", filename))
		a.schemaEntry.SetText(writer.URI().Path())
		
	}, a.window)
}

// saveRequestToFile saves the current request settings to a YAML file
func (a *App) saveRequestToFile(methodSelect *widget.Select, urlEntry, contentTypeEntry, authEntry, bodyEntry, statusEntry *widget.Entry) {
	// 파일 저장 다이얼로그
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			a.logMessage(fmt.Sprintf("❌ 파일 저장 오류: %v", err))
			return
		}
		if writer == nil {
			return // 사용자가 취소
		}
		defer writer.Close()
		
		// YAML 형식으로 요청 설정 생성
		yamlContent := a.generateRequestYAML(methodSelect, urlEntry, contentTypeEntry, authEntry, bodyEntry, statusEntry)
		
		// 파일에 쓰기
		_, err = writer.Write([]byte(yamlContent))
		if err != nil {
			a.logMessage(fmt.Sprintf("❌ 파일 쓰기 오류: %v", err))
			return
		}
		
		// 성공 메시지
		filename := writer.URI().Name()
		a.logMessage(fmt.Sprintf("✅ 요청 설정이 저장되었습니다: %s", filename))
		a.requestEntry.SetText(writer.URI().Path())
		
	}, a.window)
}

// addColumn adds a new column to the schema
func (a *App) addColumn() {
	newColumn := SchemaColumn{
		Name:     fmt.Sprintf("column_%d", len(a.schemaData.Columns)+1),
		Type:     "string",
		Required: false,
		Regex:    "",
		MinLen:   0,
		MaxLen:   0,
		Enum:     []string{},
	}
	a.schemaData.Columns = append(a.schemaData.Columns, newColumn)
}

// removeColumn removes a column from the schema
func (a *App) removeColumn(index int) {
	if index < 0 || index >= len(a.schemaData.Columns) {
		return
	}
	
	// Remove the column at the specified index
	a.schemaData.Columns = append(a.schemaData.Columns[:index], a.schemaData.Columns[index+1:]...)
	a.logMessage(fmt.Sprintf("컬럼 %d가 삭제되었습니다", index+1))
} 