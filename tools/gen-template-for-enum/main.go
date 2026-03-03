package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/samber/lo"
)

var (
	packagePath    = flag.String("path", ".", "Path to the package containing the structs")
	outputFilePath = flag.String("output", "", "Path to the output file for generated code")
	enumTypeName   = flag.String("type", "", "Type name")
	templateFile   = flag.String("tmpl_file", "", "Give template file name. Same path with package")
)

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of gen-template:\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Validate `mode` input
	if lo.FromPtr(packagePath) == "" ||
		lo.FromPtr(outputFilePath) == "" ||
		lo.FromPtr(enumTypeName) == "" ||
		lo.FromPtr(templateFile) == "" {
		fmt.Println("Invalid flags. `output` `type` `tmpl_file` Flags are required")
		Usage()
		os.Exit(1)
	}

	//
	pkg := loadPkgs(*packagePath)
	info := getType(pkg, *enumTypeName)

	err := createFileFromTemplate(*outputFilePath, *templateFile, templateData{
		PackageName:  pkg.Name,
		EnumTypeName: *enumTypeName,
		EnumValues: lo.Map(
			info.EnumValues, func(i *EnumValues, _ int) EnumValues {
				return *i
			}),
	})

	if err != nil {
		log.Printf("Get error: [%s]", err.Error())
	} else {
		log.Println("Code generation completed successfully.")
	}
}
