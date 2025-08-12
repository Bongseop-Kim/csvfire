package main

import (
	"context"
	"sync"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type AppState struct {
	SchemaFile  string
	CSVFile     string
	RequestFile string
	LogDir      string
	
	// Settings
	Concurrency int
	RateLimit   string
	Timeout     string
	Resume      bool
	ExportFailed string
	
	// Runtime
	IsRunning bool
	Cancel    context.CancelFunc
	mu        sync.RWMutex
}

// SchemaColumn represents a column in the schema editor
type SchemaColumn struct {
	Name     string
	Type     string
	Required bool
	Regex    string
	MinLen   int
	MaxLen   int
	Enum     []string
}

// SchemaData holds the current schema being edited
type SchemaData struct {
	Columns []SchemaColumn
}

// RegexPreset represents a predefined regex pattern
type RegexPreset struct {
	Name    string
	Pattern string
	Description string
}

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	state   *AppState
	
	// Schema Editor Data
	schemaData *SchemaData
	
	// UI Elements
	schemaEntry    *widget.Entry
	csvEntry       *widget.Entry
	requestEntry   *widget.Entry
	logDirEntry    *widget.Entry
	
	concurrencyEntry *widget.Entry
	rateLimitEntry   *widget.Entry
	timeoutEntry     *widget.Entry
	resumeCheck      *widget.Check
	exportEntry      *widget.Entry
	
	validateBtn *widget.Button
	renderBtn   *widget.Button
	runBtn      *widget.Button
	stopBtn     *widget.Button
	
	progressBar   *widget.ProgressBar
	statusLabel   *widget.Label
	logTextArea   *widget.Entry
}