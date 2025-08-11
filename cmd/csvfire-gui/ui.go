package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (a *App) buildUI() {
	window := a.fyneApp.NewWindow("CSVFire - CSV 기반 API 호출 도구")
	window.Resize(fyne.NewSize(1000, 700))
	
	// 상단 컴팩트 컨트롤 패널
	topPanel := a.createCompactControlPanel()
	
	// 하단 메인 로그 섹션
	logSection := a.createMainLogSection()
	
	// 상하 분할 레이아웃 (20% : 80%)
	content := container.NewBorder(
		topPanel, // top
		nil,      // bottom  
		nil,      // left
		nil,      // right
		logSection, // center (메인 로그)
	)
	
	window.SetContent(content)
	a.window = window
	
	// Initial state
	a.updateButtons()
}

func (a *App) createCompactControlPanel() *fyne.Container {
	// 파일 선택 섹션 (한 줄로)
	a.schemaEntry = widget.NewEntry()
	a.schemaEntry.SetPlaceHolder("스키마 파일")
	a.schemaEntry.Resize(fyne.NewSize(200, 30)) // 최소 너비 설정
	schemaBrowseBtn := widget.NewButton("📁", func() {
		a.browseFile("YAML 파일", []string{".yaml", ".yml"}, a.schemaEntry)
	})
	schemaConfigBtn := widget.NewButton("⚙️", func() {
		a.showSchemaConfigDialog()
	})
	
	a.csvEntry = widget.NewEntry()
	a.csvEntry.SetPlaceHolder("CSV 파일")
	a.csvEntry.Resize(fyne.NewSize(200, 30))
	csvBrowseBtn := widget.NewButton("📁", func() {
		a.browseFile("CSV 파일", []string{".csv"}, a.csvEntry)
	})
	
	a.requestEntry = widget.NewEntry()
	a.requestEntry.SetPlaceHolder("요청 설정 파일")
	a.requestEntry.Resize(fyne.NewSize(200, 30))
	requestBrowseBtn := widget.NewButton("📁", func() {
		a.browseFile("YAML 파일", []string{".yaml", ".yml"}, a.requestEntry)
	})
	requestConfigBtn := widget.NewButton("⚙️", func() {
		a.showRequestConfigDialog()
	})

	// 설정 섹션 (한 줄로)
	a.concurrencyEntry = widget.NewEntry()
	a.concurrencyEntry.SetText(strconv.Itoa(a.state.Concurrency))
	a.concurrencyEntry.Resize(fyne.NewSize(60, 30))
	
	a.rateLimitEntry = widget.NewEntry()
	a.rateLimitEntry.SetText(a.state.RateLimit)
	a.rateLimitEntry.Resize(fyne.NewSize(60, 30))
	
	a.timeoutEntry = widget.NewEntry()
	a.timeoutEntry.SetText(a.state.Timeout)
	a.timeoutEntry.Resize(fyne.NewSize(60, 30))

	// 추가 설정들
	a.resumeCheck = widget.NewCheck("재시작", func(checked bool) {
		a.state.Resume = checked
	})
	
	a.exportEntry = widget.NewEntry()
	a.exportEntry.SetPlaceHolder("실패행저장파일")
	
	a.logDirEntry = widget.NewEntry()
	a.logDirEntry.SetText(a.state.LogDir)

	// 액션 버튼들
	a.validateBtn = widget.NewButton("🔍 검증", a.onValidate)
	a.renderBtn = widget.NewButton("👁️ 미리보기", a.onRender)
	a.runBtn = widget.NewButton("🚀 실행", a.onRun)
	a.stopBtn = widget.NewButton("⏹️ 중지", a.onStop)

	// 진행률 및 상태
	a.progressBar = widget.NewProgressBar()
	a.statusLabel = widget.NewLabel("준비됨")

	// 파일 선택 섹션 (줄바꿈으로 깔끔하게)
	
	a.schemaEntry.SetPlaceHolder("스키마 파일 경로")
	a.csvEntry.SetPlaceHolder("CSV 파일 경로")  
	a.requestEntry.SetPlaceHolder("요청 설정 파일 경로")
	
	// 각 파일을 한 줄씩 배치하여 전체 너비 활용
	schemaRow := container.NewBorder(nil, nil, 
		widget.NewLabel("스키마:"), 
		container.NewHBox(schemaConfigBtn, schemaBrowseBtn), 
		a.schemaEntry)
	
	csvRow := container.NewBorder(nil, nil, 
		widget.NewLabel("CSV:"), 
		csvBrowseBtn, 
		a.csvEntry)
	
	requestRow := container.NewBorder(nil, nil, 
		widget.NewLabel("요청:"), 
		container.NewHBox(requestConfigBtn, requestBrowseBtn), 
		a.requestEntry)
	
	// VBox로 세로 배치
	fileSection := container.NewVBox(
		schemaRow,
		csvRow,
		requestRow,
	)

	// 두 번째 행: 설정 + 버튼들
	controlRow := container.NewHBox(
		widget.NewLabel("동시성:"), a.concurrencyEntry,
		widget.NewLabel("속도:"), a.rateLimitEntry,  
		widget.NewLabel("타임아웃:"), a.timeoutEntry,
		widget.NewSeparator(),
		a.validateBtn, a.renderBtn, a.runBtn, a.stopBtn,
	)

	// 세 번째 행: 진행률
	progressRow := container.NewBorder(
		nil, nil, a.statusLabel, nil, a.progressBar,
	)

	return container.NewVBox(
		fileSection,
		controlRow, 
		progressRow,
		widget.NewSeparator(),
	)
}

func (a *App) createMainLogSection() *fyne.Container {
	// 메인 로그 영역
	a.logTextArea = widget.NewMultiLineEntry()
	a.logTextArea.SetText("📝 실시간 로그\n" + 
		strings.Repeat("=", 50) + "\n" +
		"로그가 여기에 실시간으로 표시됩니다...\n\n")
	a.logTextArea.Wrapping = fyne.TextWrapWord
	
	// 로그 컨트롤 버튼들
	clearBtn := widget.NewButton("🗑️ 지우기", func() {
		a.logTextArea.SetText("")
	})
	
	saveBtn := widget.NewButton("💾 저장", func() {
		// TODO: 로그 저장 기능
		a.logMessage("로그 저장 기능은 추후 구현 예정")
	})
	
	pauseBtn := widget.NewButton("⏸️ 일시정지", func() {
		// TODO: 로그 일시정지 기능  
		a.logMessage("로그 일시정지 기능은 추후 구현 예정")
	})

	// 로그 컨트롤 툴바
	logControls := container.NewHBox(
		widget.NewLabel("📝 실시간 로그"),
		layout.NewSpacer(),
		pauseBtn, clearBtn, saveBtn,
	)

	// 로그 영역 (스크롤 가능)
	logScroll := container.NewScroll(a.logTextArea)
	
	return container.NewBorder(
		logControls, // top
		nil,         // bottom
		nil,         // left  
		nil,         // right
		logScroll,   // center
	)
}

func (a *App) updateButtons() {
	a.state.mu.RLock()
	isRunning := a.state.IsRunning
	a.state.mu.RUnlock()
	
	a.validateBtn.Enable()
	a.renderBtn.Enable()
	
	if isRunning {
		a.runBtn.Disable()
		a.stopBtn.Enable()
	} else {
		a.runBtn.Enable()
		a.stopBtn.Disable()
	}
}

func (a *App) logMessage(message string) {
	timestamp := time.Now().Format("15:04:05")
	logLine := fmt.Sprintf("[%s] %s\n", timestamp, message)
	
	// Fyne 스레드 안전성을 위해 fyne.Do 사용
	fyne.Do(func() {
		currentText := a.logTextArea.Text
		a.logTextArea.SetText(currentText + logLine)
		
		// 자동 스크롤 (마지막 줄로)
		if len(currentText) > 0 {
			a.logTextArea.CursorRow = len(strings.Split(currentText, "\n"))
		}
	})
}

func (a *App) setStatus(status string) {
	fyne.Do(func() {
		a.statusLabel.SetText(status)
	})
}