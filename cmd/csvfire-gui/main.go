package main

import (
	"fyne.io/fyne/v2/app"
)

func main() {
	myApp := app.NewWithID("io.csvfire.app")
	myApp.SetIcon(nil) // You can add an icon here
	
	appState := &AppState{
		LogDir:      "logs",
		Concurrency: 8,
		RateLimit:   "5/s",
		Timeout:     "10s",
		Resume:      false,
	}
	
	mainApp := &App{
		fyneApp: myApp,
		state:   appState,
		schemaData: &SchemaData{
			Columns: []SchemaColumn{},
		},
	}
	
	mainApp.buildUI()
	mainApp.showAndRun()
}

func (a *App) showAndRun() {
	a.window.ShowAndRun()
} 








