package main

import (
	"log"

	"github.com/fatih/color"
)

var (
	infoLog  = color.New(color.FgCyan).SprintFunc()
	errorLog = color.New(color.FgRed).SprintFunc()
)

func Info(message string) {
	log.Println(infoLog("[INFO]"), message)
}

func Error(message string) {
	log.Println(errorLog("[ERROR]"), message)
}
