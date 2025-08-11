# CSVFire

CSV의 각 행을 파라미터로 API를 반복 호출하고, 사전검증 및 요청/응답 로그를 CSV로 남기는 도구입니다.

## 주요 기능

- **스키마 기반 검증**: YAML 스키마로 데이터 타입, 필수값, 정규화 규칙 정의
- **요청 템플릿**: Go text/template 기반 HTTP 요청 템플릿
- **동시성 및 레이트 리밋**: 워커풀과 레이트 리밋으로 성능 제어
- **재시도 및 복구**: 네트워크 오류와 5xx 에러에 대한 자동 재시도
- **로깅**: 요청/응답을 CSV 형태로 상세 로깅
- **재시작 지원**: 실패한 지점부터 재시작 가능
- **민감정보 보호**: secret 컬럼 자동 마스킹

## 설치

### 소스에서 빌드

```bash
git clone <repository>
cd csvfire
go mod tidy
go build -o csvfire cmd/csvfire/main.go
```

### 바이너리 실행

```bash
./csvfire --help
```

## 사용법

### 1. validate - 데이터 검증

CSV 데이터를 스키마에 따라 검증합니다.

```bash
./csvfire validate --schema samples/schema.yaml --csv samples/data.csv --report logs/validate_errors.csv --strict
```

**옵션:**

- `--schema`: 스키마 파일 경로 (필수)
- `--csv`: CSV 파일 경로 (필수)
- `--report`: 검증 오류 리포트 파일 (기본값: logs/validate_errors.csv)
- `--strict`: 검증 실패시 종료 코드 1로 종료

### 2. render - 요청 미리보기

실제 전송 없이 요청 템플릿을 렌더링하여 미리보기를 생성합니다.

```bash
./csvfire render --schema samples/schema.yaml --csv samples/data.csv --request samples/request.yaml --limit 10 --preview logs/preview.jsonl
```

**옵션:**

- `--schema`: 스키마 파일 경로 (필수)
- `--csv`: CSV 파일 경로 (필수)
- `--request`: 요청 설정 파일 경로 (필수)
- `--limit`: 미리보기할 행 수 (기본값: 10)
- `--preview`: 미리보기 파일 경로 (기본값: logs/preview.jsonl)

### 3. run - 실제 API 호출

검증된 데이터로 실제 API 호출을 실행합니다.

```bash
./csvfire run --schema samples/schema.yaml --csv samples/data.csv --request samples/request.yaml --concurrency 8 --rate 5/s --timeout 10s --log logs --export-failed failed_rows.csv
```

**옵션:**

- `--schema`: 스키마 파일 경로 (필수)
- `--csv`: CSV 파일 경로 (필수)
- `--request`: 요청 설정 파일 경로 (필수)
- `--concurrency`: 동시 요청 수 (기본값: 8)
- `--rate`: 요청 속도 제한, 예: 5/s
- `--timeout`: 요청 타임아웃 (기본값: 10s)
- `--log`: 로그 디렉토리 (기본값: logs)
- `--export-failed`: 실패한 행을 내보낼 파일
- `--resume`: 이전 실행 재시작

## 설정 파일 형식

### 스키마 파일 (schema.yaml)

```yaml
version: 1
columns:
  - name: name
    type: string
    required: true
    min_len: 1
    max_len: 50
    validators:
      - regex: "^[가-힣a-zA-Z\\s]+$"
        message: "이름은 한글, 영문, 공백만 허용됩니다"

  - name: phone
    type: string
    required: true
    preprocess:
      - remove: ["-", " ", "\t"]
        trim: true
    validators:
      - regex: "^[0-9]{10,11}$"
        message: "휴대폰번호는 10~11자리 숫자여야 합니다"
    transform:
      - format_korean_phone_e164: true

  - name: birth
    type: date
    required: true
    format: "20060102"

  - name: gender
    type: string
    required: true
    enum: ["M", "F", "1", "2"]
    normalize:
      map:
        "1": "M"
        "2": "F"

  - name: token
    type: string
    required: false
    secret: true

row_rules:
  - name: age_range
    expr: "age(birth) >= 0 && age(birth) <= 120"

uniqueness:
  - columns: ["phone"]

null_policy:
  treat_empty_as_null: true
```

**지원하는 데이터 타입:**

- `string`: 문자열
- `int`: 정수
- `float`: 실수
- `decimal(precision,scale)`: 고정소수점
- `date`: 날짜 (기본 형식: YYYYMMDD)

**검증 규칙:**

- `required`: 필수 필드
- `min_len`, `max_len`: 문자열 길이 제한
- `regex`: 정규표현식 검증
- `enum`: 허용값 목록
- `range`: 숫자 범위 (min, max)

**전처리:**

- `remove`: 특정 문자 제거
- `replace`: 문자 치환
- `trim`: 공백 제거

**정규화:**

- `map`: 값 매핑

**변환:**

- `format_korean_phone_e164`: 한국 휴대폰번호를 E164 형식으로 변환

### 요청 설정 파일 (request.yaml)

```yaml
method: POST
url: "https://api.example.com/users"
headers:
  Content-Type: "application/json"
  Authorization: "{{ if .token }}Bearer {{.token}}{{ end }}"
proxy: "{{ .proxy }}"
body: |
  {
    "name": "{{ .name }}",
    "phone": "{{ .phone }}",
    "birth": "{{ .birth }}",
    "gender": "{{ .gender }}"
  }
success:
  status_in: [200, 201]
  response_keys:
    status: "success"
```

**템플릿 함수:**

- `dateFormat`: 날짜 형식 변환
- `toE164KR`: 한국 휴대폰번호 E164 변환
- `mask`: 민감정보 마스킹
- `hash`: SHA256 해시
- `upper`, `lower`: 대소문자 변환
- `trim`: 공백 제거

## 출력 파일

### 로그 파일

#### logs/sent.csv

모든 요청의 상세 로그

```csv
ts,row,request_id,status_code,success,latency_ms,retries,error_category,error_detail,response_preview,request_hash
```

#### logs/request_errors.csv

실패한 요청 요약

```csv
ts,row,request_id,error_category,error_detail,status_code
```

#### logs/validate_errors.csv

검증 오류 상세

```csv
ts,row,column,value,message
```

### 실패한 행 파일 (failed_rows.csv)

실패한 행을 원본 형식으로 추출하여 재처리 가능

## 성능 최적화

- **동시성**: `--concurrency` 옵션으로 동시 요청 수 조절
- **레이트 리밋**: `--rate` 옵션으로 API 서버 부하 제어
- **스트리밍**: 대용량 CSV도 메모리 효율적 처리
- **재시작**: `--resume` 옵션으로 중단된 작업 재시작

## 제한사항

- CSV 파일은 헤더 행이 있어야 함
- 스키마의 컬럼 순서와 CSV 헤더 순서가 일치해야 함
- 행 단위 표현식 평가는 제한적 (현재 age() 함수만 지원)
- 바이너리 데이터는 지원하지 않음

## 예제

전체 워크플로우 예제:

```bash
# 1. 데이터 검증
./csvfire validate --schema samples/schema.yaml --csv samples/data.csv --strict

# 2. 요청 미리보기
./csvfire render --schema samples/schema.yaml --csv samples/data.csv --request samples/request.yaml --limit 5

# 3. 실제 실행
./csvfire run --schema samples/schema.yaml --csv samples/data.csv --request samples/request.yaml --concurrency 4 --rate 2/s --timeout 15s --export-failed failed.csv
```

## 라이선스

이 프로젝트는 MIT 라이선스 하에 배포됩니다.
