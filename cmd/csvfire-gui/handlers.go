package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"csvfire/internal/config"
	"csvfire/internal/logger"
	"csvfire/internal/reader"
	"csvfire/internal/request"
	"csvfire/internal/runner"
	"csvfire/internal/validator"
)

func (a *App) browseFile(title string, extensions []string, entry *widget.Entry) {
    fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
        if err == nil && reader != nil {
            entry.SetText(reader.URI().Path())
            reader.Close()
        }
    }, a.window)
    if len(extensions) > 0 {
        fd.SetFilter(storage.NewExtensionFileFilter(extensions))
    }
    fd.SetConfirmText("선택")
    fd.Show()
}

func (a *App) updateState() {
	a.state.SchemaFile = a.schemaEntry.Text
	a.state.CSVFile = a.csvEntry.Text
	a.state.RequestFile = a.requestEntry.Text
	a.state.LogDir = a.logDirEntry.Text
	a.state.ExportFailed = a.exportEntry.Text
	if a.resumeCheck != nil {
		a.state.Resume = a.resumeCheck.Checked
	}

	if concurrency, err := strconv.Atoi(a.concurrencyEntry.Text); err == nil {
		a.state.Concurrency = concurrency
	} else {
		a.state.Concurrency = 1
	}
	a.state.RateLimit = a.rateLimitEntry.Text
	a.state.Timeout = a.timeoutEntry.Text
}

func (a *App) onValidate() {
	a.updateState()
	
	if a.state.SchemaFile == "" || a.state.CSVFile == "" {
		dialog.ShowError(fmt.Errorf("스키마 파일과 CSV 파일을 선택해주세요"), a.window)
		return
	}
	
	go func() {
		a.logMessage("검증을 시작합니다...")
		a.setStatus("검증 중...")
		
		// Load schema
		schema, err := config.LoadSchema(a.state.SchemaFile)
		if err != nil {
			a.logMessage(fmt.Sprintf("스키마 로드 실패: %v", err))
			a.setStatus("검증 실패")
			return
		}
		
		// Create CSV reader
		csvReader := reader.NewCSVReader(schema, a.state.CSVFile)
		
		// Create validator
		val := validator.NewValidator(schema)
		
		// Read and validate using streaming approach
		totalErrors := 0
		loggedErrors := 0
		totalRows, validRows, errorCount, err := csvReader.ValidateRowsStream(func(rowNum int, data map[string]string) (bool, []error) {
			result := val.ValidateRow(rowNum, data)
			
			// Always count total errors
			if !result.Valid {
				totalErrors += len(result.Errors)
				
				// Log up to 5 errors total
				remainingLogSlots := 5 - loggedErrors
				if remainingLogSlots < 0 {
					remainingLogSlots = 0
				}
				errorsToLog := len(result.Errors)
				if errorsToLog > remainingLogSlots {
					errorsToLog = remainingLogSlots
				}
				
				for i := 0; i < errorsToLog; i++ {
					err := result.Errors[i]
					a.logMessage(fmt.Sprintf("행 %d, 컬럼 %s: %s", err.Row, err.Column, err.Message))
					loggedErrors++
				}
			}
			
			var errors []error
			if !result.Valid {
				for _, validationErr := range result.Errors {
					errors = append(errors, fmt.Errorf("%s", validationErr.Message))
				}
			}
			
			return result.Valid, errors
		})
		
		if err != nil {
			a.logMessage(fmt.Sprintf("CSV 읽기/검증 실패: %v", err))
			a.setStatus("검증 실패")
			return
		}
		
		if loggedErrors < errorCount {
			a.logMessage(fmt.Sprintf("검증 완료 - 총: %d, 유효: %d, 오류: %d (첫 %d개 오류만 표시됨)", totalRows, validRows, errorCount, loggedErrors))
		} else {
			a.logMessage(fmt.Sprintf("검증 완료 - 총: %d, 유효: %d, 오류: %d", totalRows, validRows, errorCount))
		}
		a.setStatus(fmt.Sprintf("검증 완료: %d/%d 성공", validRows, totalRows))
	}()
}

func (a *App) onRender() {
	a.updateState()
	
	if a.state.SchemaFile == "" || a.state.CSVFile == "" || a.state.RequestFile == "" {
		dialog.ShowError(fmt.Errorf("모든 파일을 선택해주세요"), a.window)
		return
	}
	
	go func() {
		a.logMessage("템플릿 미리보기를 생성합니다...")
		a.setStatus("미리보기 생성 중...")
		
		// Load configurations
		schema, err := config.LoadSchema(a.state.SchemaFile)
		if err != nil {
			a.logMessage(fmt.Sprintf("스키마 로드 실패: %v", err))
			a.setStatus("미리보기 실패")
			return
		}
		
		requestConfig, err := config.LoadRequestConfig(a.state.RequestFile)
		if err != nil {
			a.logMessage(fmt.Sprintf("요청 설정 로드 실패: %v", err))
			a.setStatus("미리보기 실패")
			return
		}
		
		// Create components
		csvReader := reader.NewCSVReader(schema, a.state.CSVFile)
		renderer, err := request.NewTemplateRenderer(requestConfig)
		if err != nil {
			a.logMessage(fmt.Sprintf("템플릿 렌더러 생성 실패: %v", err))
			a.setStatus("미리보기 실패")
			return
		}
		
		val := validator.NewValidator(schema)
		
		// Preview first 3 rows
		rows, err := csvReader.GetPreviewRows(3)
		if err != nil {
			a.logMessage(fmt.Sprintf("CSV 읽기 실패: %v", err))
			a.setStatus("미리보기 실패")
			return
		}
		
		processedCount := 0
		for i, row := range rows {
			result := val.ValidateRow(i+1, row)
			if !result.Valid {
				a.logMessage(fmt.Sprintf("행 %d: 검증 실패", i+1))
				continue
			}
			
			requestData, err := renderer.Render(result.Data)
			if err != nil {
				a.logMessage(fmt.Sprintf("행 %d: 템플릿 렌더링 실패: %v", i+1, err))
				continue
			}
			
			a.logMessage(fmt.Sprintf("행 %d: %s %s", i+1, requestData.Method, requestData.URL))
			a.logMessage(fmt.Sprintf("  Body: %s", strings.ReplaceAll(requestData.Body, "\n", " ")))
			processedCount++
		}
		
		a.logMessage(fmt.Sprintf("미리보기 완료: %d행 처리", processedCount))
		a.setStatus(fmt.Sprintf("미리보기 완료: %d행", processedCount))
	}()
}

func (a *App) onRun() {
	a.updateState()
	
	if a.state.SchemaFile == "" || a.state.CSVFile == "" || a.state.RequestFile == "" {
		dialog.ShowError(fmt.Errorf("모든 파일을 선택해주세요"), a.window)
		return
	}
	
	a.state.mu.Lock()
	a.state.IsRunning = true
	a.state.mu.Unlock()
	a.updateButtons()
	
	go func() {
		defer func() {
			a.state.mu.Lock()
			a.state.IsRunning = false
			a.state.Cancel = nil
			a.state.mu.Unlock()
			a.updateButtons()
		}()
		
		a.logMessage("API 호출 실행을 시작합니다...")
		a.setStatus("실행 중...")
		a.progressBar.SetValue(0)
		
		// Parse settings
		timeout, err := time.ParseDuration(a.state.Timeout)
		if err != nil {
			a.logMessage(fmt.Sprintf("타임아웃 파싱 실패: %v", err))
			a.setStatus("실행 실패")
			return
		}
		
		var rateLimitValue float64
		if a.state.RateLimit != "" && strings.HasSuffix(a.state.RateLimit, "/s") {
			rateStr := strings.TrimSuffix(a.state.RateLimit, "/s")
			rateLimitValue, err = strconv.ParseFloat(rateStr, 64)
			if err != nil {
				a.logMessage(fmt.Sprintf("레이트 리밋 파싱 실패: %v", err))
				a.setStatus("실행 실패")
				return
			}
		}
		
		// Load configurations
		schema, err := config.LoadSchema(a.state.SchemaFile)
		if err != nil {
			a.logMessage(fmt.Sprintf("스키마 로드 실패: %v", err))
			a.setStatus("실행 실패")
			return
		}
		
		requestConfig, err := config.LoadRequestConfig(a.state.RequestFile)
		if err != nil {
			a.logMessage(fmt.Sprintf("요청 설정 로드 실패: %v", err))
			a.setStatus("실행 실패")
			return
		}
		
		// Create runner
		runConfig := &runner.RunConfig{
			Concurrency: a.state.Concurrency,
			RateLimit:   rateLimitValue,
			Timeout:     timeout,
			Resume:      a.state.Resume,
		}
		
		runnerInstance, err := runner.NewRunner(schema, requestConfig, runConfig)
		if err != nil {
			a.logMessage(fmt.Sprintf("런너 생성 실패: %v", err))
			a.setStatus("실행 실패")
			return
		}
		
		// Create logger
		loggerInstance, err := logger.NewLogger(schema, a.state.LogDir)
		if err != nil {
			a.logMessage(fmt.Sprintf("로거 생성 실패: %v", err))
			a.setStatus("실행 실패")
			return
		}
		defer loggerInstance.Close()
		
		// Create context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // Ensure context is always canceled
		a.state.mu.Lock()
		a.state.Cancel = cancel
		a.state.mu.Unlock()
		
		// Create CSV reader
		csvReader := reader.NewCSVReader(schema, a.state.CSVFile)
		
		// Count total rows for progress tracking
		totalRows, err := csvReader.CountRows()
		if err != nil {
			a.logMessage(fmt.Sprintf("행 수 계산 실패: %v", err))
			totalRows = 0
		}
		
		// Create task channel
		tasksChan := make(chan runner.RowTask, a.state.Concurrency*2)
		
		// Start CSV reading
		go func() {
			if err := csvReader.ReadRows(tasksChan); err != nil {
				a.logMessage(fmt.Sprintf("CSV 읽기 오류: %v", err))
				cancel()
			}
		}()
		
		// Progress tracking
		var processedCount int32
		
		// Result callback
		callback := func(rowNum int, validationResult *validator.ValidationResult, requestResult *request.RequestResult) {
			loggerInstance.LogRequest(rowNum, validationResult, requestResult)
			
			processedCount++
			if totalRows > 0 {
				progress := float64(processedCount) / float64(totalRows)
				a.progressBar.SetValue(progress)
			}
			
			if requestResult != nil {
				if requestResult.Success {
					a.logMessage(fmt.Sprintf("행 %d: 성공 (상태: %d)", rowNum, requestResult.StatusCode))
				} else {
					a.logMessage(fmt.Sprintf("행 %d: 실패 (%s)", rowNum, requestResult.ErrorCategory))
				}
			} else {
				a.logMessage(fmt.Sprintf("행 %d: 검증 실패", rowNum))
			}
			
			a.setStatus(fmt.Sprintf("처리 중: %d행 완료", processedCount))
		}
		
		// Execute
		result := runnerInstance.Run(ctx, tasksChan, callback)
		
		// Final results
		a.progressBar.SetValue(1.0)
		a.logMessage(fmt.Sprintf("실행 완료 - 총: %d, 성공: %d, 실패: %d", 
			result.TotalRows, result.SuccessRows, result.FailedRows))
		a.setStatus(fmt.Sprintf("완료: 성공 %d, 실패 %d", result.SuccessRows, result.FailedRows))
		
		// Export failed rows if requested
		if a.state.ExportFailed != "" && loggerInstance.GetFailedRowCount() > 0 {
			if err := loggerInstance.ExportFailedRows(a.state.ExportFailed); err != nil {
				a.logMessage(fmt.Sprintf("실패한 행 내보내기 오류: %v", err))
			} else {
				a.logMessage(fmt.Sprintf("실패한 행 저장됨: %s", a.state.ExportFailed))
			}
		}
	}()
}

func (a *App) onStop() {
	a.state.mu.Lock()
	if a.state.Cancel != nil {
		a.state.Cancel()
		a.logMessage("중지 요청됨...")
		a.setStatus("중지 중...")
	}
	a.state.mu.Unlock()
} 
