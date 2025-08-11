package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"csvfire/internal/config"
	"csvfire/internal/logger"
	"csvfire/internal/reader"
	"csvfire/internal/request"
	"csvfire/internal/runner"
	"csvfire/internal/validator"
)

var (
	schemaFile    string
	csvFile       string
	requestFile   string
	reportFile    string
	logDir        string
	exportFailed  string
	concurrency   int
	rateLimit     string
	timeoutStr    string
	strict        bool
	resume        bool
	limit         int
	previewFile   string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "csvfire",
		Short: "CSV 행 기반 API 호출 도구",
		Long:  "CSV의 각 행을 파라미터로 API를 반복 호출하고, 사전검증 및 요청/응답 로그를 CSV로 남기는 도구",
	}

	// validate 서브커맨드
	var validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "CSV 데이터 검증",
		Long:  "스키마 기반으로 CSV 데이터를 검증하고 오류를 리포트합니다",
		RunE:  runValidate,
	}

	validateCmd.Flags().StringVar(&schemaFile, "schema", "", "스키마 파일 경로 (schema.yaml)")
	validateCmd.Flags().StringVar(&csvFile, "csv", "", "CSV 파일 경로")
	validateCmd.Flags().StringVar(&reportFile, "report", "logs/validate_errors.csv", "검증 오류 리포트 파일")
	validateCmd.Flags().BoolVar(&strict, "strict", false, "검증 실패 시 종료 코드 1로 종료")
	validateCmd.MarkFlagRequired("schema")
	validateCmd.MarkFlagRequired("csv")

	// render 서브커맨드
	var renderCmd = &cobra.Command{
		Use:   "render",
		Short: "요청 템플릿 미리보기",
		Long:  "요청 템플릿을 렌더링하여 미리보기를 생성합니다 (실제 전송하지 않음)",
		RunE:  runRender,
	}

	renderCmd.Flags().StringVar(&schemaFile, "schema", "", "스키마 파일 경로")
	renderCmd.Flags().StringVar(&csvFile, "csv", "", "CSV 파일 경로")
	renderCmd.Flags().StringVar(&requestFile, "request", "", "요청 설정 파일 경로")
	renderCmd.Flags().IntVar(&limit, "limit", 10, "미리보기할 행 수")
	renderCmd.Flags().StringVar(&previewFile, "preview", "logs/preview.jsonl", "미리보기 파일 경로")
	renderCmd.MarkFlagRequired("schema")
	renderCmd.MarkFlagRequired("csv")
	renderCmd.MarkFlagRequired("request")

	// run 서브커맨드
	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "실제 API 호출 실행",
		Long:  "검증된 CSV 데이터로 실제 API 호출을 실행합니다",
		RunE:  runExecute,
	}

	runCmd.Flags().StringVar(&schemaFile, "schema", "", "스키마 파일 경로")
	runCmd.Flags().StringVar(&csvFile, "csv", "", "CSV 파일 경로")
	runCmd.Flags().StringVar(&requestFile, "request", "", "요청 설정 파일 경로")
	runCmd.Flags().IntVar(&concurrency, "concurrency", 8, "동시 요청 수")
	runCmd.Flags().StringVar(&rateLimit, "rate", "", "요청 속도 제한 (예: 5/s)")
	runCmd.Flags().StringVar(&timeoutStr, "timeout", "10s", "요청 타임아웃")
	runCmd.Flags().StringVar(&logDir, "log", "logs", "로그 디렉토리")
	runCmd.Flags().StringVar(&exportFailed, "export-failed", "", "실패한 행을 내보낼 파일")
	runCmd.Flags().BoolVar(&resume, "resume", false, "이전 실행 재시작")
	runCmd.MarkFlagRequired("schema")
	runCmd.MarkFlagRequired("csv")
	runCmd.MarkFlagRequired("request")

	rootCmd.AddCommand(validateCmd, renderCmd, runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	// 스키마 로드
	schema, err := config.LoadSchema(schemaFile)
	if err != nil {
		return fmt.Errorf("스키마 로드 실패: %w", err)
	}

	// CSV 리더 생성
	csvReader := reader.NewCSVReader(schema, csvFile)

	// 검증기 생성
	val := validator.NewValidator(schema)

	// 리포트 디렉토리 생성
	if reportFile != "" {
		reportDir := filepath.Dir(reportFile)
		if err := os.MkdirAll(reportDir, 0755); err != nil {
			return fmt.Errorf("리포트 디렉토리 생성 실패: %w", err)
		}
	}

	fmt.Printf("CSV 검증을 시작합니다: %s\n", csvFile)
	fmt.Printf("스키마: %s\n", schemaFile)

	// 미리보기로 모든 행 읽기
	rows, err := csvReader.GetPreviewRows(1000000) // 충분히 큰 수로 모든 행 읽기
	if err != nil {
		return fmt.Errorf("CSV 읽기 실패: %w", err)
	}

	totalRows := len(rows)
	validRows := 0
	errorCount := 0

	// 검증 오류 수집
	var allErrors []validator.ValidationError

	for i, row := range rows {
		result := val.ValidateRow(i+1, row)
		if result.Valid {
			validRows++
		} else {
			errorCount += len(result.Errors)
			allErrors = append(allErrors, result.Errors...)
		}
	}

	// 리포트 작성
	if len(allErrors) > 0 && reportFile != "" {
		if err := writeValidationReport(reportFile, allErrors); err != nil {
			return fmt.Errorf("리포트 작성 실패: %w", err)
		}
		fmt.Printf("검증 오류 리포트: %s\n", reportFile)
	}

	// 결과 출력
	fmt.Printf("\n=== 검증 결과 ===\n")
	fmt.Printf("총 행 수: %d\n", totalRows)
	fmt.Printf("유효한 행: %d\n", validRows)
	fmt.Printf("오류 행: %d\n", totalRows-validRows)
	fmt.Printf("총 오류 수: %d\n", errorCount)

	if len(allErrors) > 0 {
		fmt.Printf("\n처음 5개 오류:\n")
		for i, err := range allErrors {
			if i >= 5 {
				break
			}
			fmt.Printf("  행 %d, 컬럼 %s: %s\n", err.Row, err.Column, err.Message)
		}
	}

	if strict && len(allErrors) > 0 {
		os.Exit(1)
	}

	return nil
}

func runRender(cmd *cobra.Command, args []string) error {
	// 설정 로드
	schema, err := config.LoadSchema(schemaFile)
	if err != nil {
		return fmt.Errorf("스키마 로드 실패: %w", err)
	}

	requestConfig, err := config.LoadRequestConfig(requestFile)
	if err != nil {
		return fmt.Errorf("요청 설정 로드 실패: %w", err)
	}

	// CSV 리더 생성
	csvReader := reader.NewCSVReader(schema, csvFile)

	// 템플릿 렌더러 생성
	renderer, err := request.NewTemplateRenderer(requestConfig)
	if err != nil {
		return fmt.Errorf("템플릿 렌더러 생성 실패: %w", err)
	}

	// 검증기 생성
	val := validator.NewValidator(schema)

	fmt.Printf("요청 템플릿 미리보기를 생성합니다\n")
	fmt.Printf("제한: %d행\n", limit)

	// 미리보기 행 읽기
	rows, err := csvReader.GetPreviewRows(limit)
	if err != nil {
		return fmt.Errorf("CSV 읽기 실패: %w", err)
	}

	// 미리보기 파일 디렉토리 생성
	previewDir := filepath.Dir(previewFile)
	if err := os.MkdirAll(previewDir, 0755); err != nil {
		return fmt.Errorf("미리보기 디렉토리 생성 실패: %w", err)
	}

	// 미리보기 파일 생성
	file, err := os.Create(previewFile)
	if err != nil {
		return fmt.Errorf("미리보기 파일 생성 실패: %w", err)
	}
	defer file.Close()

	processedCount := 0
	for i, row := range rows {
		// 검증
		result := val.ValidateRow(i+1, row)
		if !result.Valid {
			fmt.Printf("행 %d: 검증 실패 (건너뛰기)\n", i+1)
			continue
		}

		// 템플릿 렌더링
		requestData, err := renderer.Render(result.Data)
		if err != nil {
			fmt.Printf("행 %d: 템플릿 렌더링 실패: %v\n", i+1, err)
			continue
		}

		// JSON으로 직렬화
		jsonData, err := json.Marshal(map[string]interface{}{
			"row":     i + 1,
			"method":  requestData.Method,
			"url":     requestData.URL,
			"headers": requestData.Headers,
			"body":    requestData.Body,
			"proxy":   requestData.Proxy,
		})
		if err != nil {
			fmt.Printf("행 %d: JSON 직렬화 실패: %v\n", i+1, err)
			continue
		}

		// 파일에 쓰기
		file.Write(jsonData)
		file.Write([]byte("\n"))
		processedCount++

		fmt.Printf("행 %d: 렌더링 완료\n", i+1)
	}

	fmt.Printf("\n미리보기 완료: %d행 처리됨\n", processedCount)
	fmt.Printf("결과 파일: %s\n", previewFile)

	return nil
}

func runExecute(cmd *cobra.Command, args []string) error {
	// 설정 로드
	schema, err := config.LoadSchema(schemaFile)
	if err != nil {
		return fmt.Errorf("스키마 로드 실패: %w", err)
	}

	requestConfig, err := config.LoadRequestConfig(requestFile)
	if err != nil {
		return fmt.Errorf("요청 설정 로드 실패: %w", err)
	}

	// 타임아웃 파싱
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return fmt.Errorf("타임아웃 파싱 실패: %w", err)
	}

	// 레이트 리밋 파싱
	var rateLimitValue float64
	if rateLimit != "" {
		if strings.HasSuffix(rateLimit, "/s") {
			rateStr := strings.TrimSuffix(rateLimit, "/s")
			rateLimitValue, err = strconv.ParseFloat(rateStr, 64)
			if err != nil {
				return fmt.Errorf("레이트 리밋 파싱 실패: %w", err)
			}
		} else {
			return fmt.Errorf("레이트 리밋 형식이 잘못됨 (예: 5/s)")
		}
	}

	// 런너 설정
	runConfig := &runner.RunConfig{
		Concurrency: concurrency,
		RateLimit:   rateLimitValue,
		Timeout:     timeout,
		Resume:      resume,
	}

	// 런너 생성
	runnerInstance, err := runner.NewRunner(schema, requestConfig, runConfig)
	if err != nil {
		return fmt.Errorf("런너 생성 실패: %w", err)
	}

	// 로거 생성
	loggerInstance, err := logger.NewLogger(schema, logDir)
	if err != nil {
		return fmt.Errorf("로거 생성 실패: %w", err)
	}
	defer loggerInstance.Close()

	// CSV 리더 생성
	csvReader := reader.NewCSVReader(schema, csvFile)

	fmt.Printf("API 호출 실행을 시작합니다\n")
	fmt.Printf("동시성: %d\n", concurrency)
	if rateLimitValue > 0 {
		fmt.Printf("레이트 리밋: %.1f/s\n", rateLimitValue)
	}
	fmt.Printf("타임아웃: %v\n", timeout)

	// 컨텍스트 설정 (Ctrl+C 처리)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\n중단 신호 수신, 정리 중...\n")
		cancel()
	}()

	// 태스크 채널 생성
	tasksChan := make(chan runner.RowTask, concurrency*2)

	// CSV 읽기 시작
	go func() {
		if err := csvReader.ReadRows(tasksChan); err != nil {
			fmt.Printf("CSV 읽기 오류: %v\n", err)
			cancel()
		}
	}()

	// 결과 콜백
	callback := func(rowNum int, validationResult *validator.ValidationResult, requestResult *request.RequestResult) {
		loggerInstance.LogRequest(rowNum, validationResult, requestResult)
		
		if requestResult != nil {
			if requestResult.Success {
				fmt.Printf("행 %d: 성공 (상태: %d, 지연: %dms)\n", 
					rowNum, requestResult.StatusCode, requestResult.LatencyMs)
			} else {
				fmt.Printf("행 %d: 실패 (%s)\n", 
					rowNum, requestResult.ErrorCategory)
			}
		} else {
			fmt.Printf("행 %d: 검증 실패\n", rowNum)
		}
	}

	// 실행
	result := runnerInstance.Run(ctx, tasksChan, callback)

	// 결과 출력
	fmt.Printf("\n=== 실행 결과 ===\n")
	fmt.Printf("총 행 수: %d\n", result.TotalRows)
	fmt.Printf("성공: %d\n", result.SuccessRows)
	fmt.Printf("실패: %d\n", result.FailedRows)
	fmt.Printf("건너뛴 행: %d\n", result.SkippedRows)
	fmt.Printf("실행 시간: %v\n", result.Duration)

	// 실패한 행 내보내기
	if exportFailed != "" && loggerInstance.GetFailedRowCount() > 0 {
		if err := loggerInstance.ExportFailedRows(exportFailed); err != nil {
			fmt.Printf("실패한 행 내보내기 오류: %v\n", err)
		} else {
			fmt.Printf("실패한 행 내보냄: %s\n", exportFailed)
		}
	}

	return nil
}

func writeValidationReport(filename string, errors []validator.ValidationError) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 헤더 쓰기
	header := []string{"timestamp", "row", "column", "value", "message"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 오류 쓰기
	for _, validationError := range errors {
		record := []string{
			time.Now().Format(time.RFC3339),
			fmt.Sprintf("%d", validationError.Row),
			validationError.Column,
			validationError.Value,
			validationError.Message,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
} 