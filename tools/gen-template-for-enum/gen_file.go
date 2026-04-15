package main

import (
	"bytes"
	"log"
	"os"
	"text/template"

	"go/format"

	"github.com/Masterminds/sprig/v3"
)

// Create a file using the provided template and data
func readTemplateFile(templatePath string) (templateString string) {
	b, err := os.ReadFile(templatePath)
	if err != nil {
		log.Fatalf("Can not open file %s", templatePath)
	}
	return string(b)
}

type templateData struct {
	PackageName  string
	EnumTypeName string
	EnumValues   []EnumValues
}

// Create a file using the provided template and data
func createFileFromTemplate(outputPath string, templatePath string, templateData templateData) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tmpl, err := template.New("mygen").Funcs(sprig.FuncMap()).Parse(readTemplateFile(templatePath))
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	tmpl.Execute(buf, templateData)

	src, err := format.Source(buf.Bytes())
	if err != nil {
		log.Printf("warning: internal error: invalid Go generated: %s", err)
		log.Printf("warning: compile the package to analyze the error")
		file.Write(buf.Bytes())
	} else {
		_, err = file.Write(src)
	}

	return err
}
