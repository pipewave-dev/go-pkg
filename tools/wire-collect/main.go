package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type wireVar struct {
	ImportPath string
	Alias      string
}

var isAliasDuplicate map[string]int

const (
	defaultVarPrefix   = "WireSet"
	defaultWireSetName = "Default"
)

var (
	project_path   = flag.String("path", ".", "Path to the package containing the structs")
	outputDirPath  = flag.String("outputDir", "", "Path to the output directory for generated code")
	genPackageName = flag.String("gen-package", "wirecollection", "Name of the generated 	package")
	varPrefix      = flag.String("var-prefix", defaultVarPrefix, "Prefix for the collected wire vars")
)

func main() {
	flag.Parse()
	if *project_path == "" || *outputDirPath == "" {
		fmt.Println("Usage: genwireset -path <project_path> -output <output_file>")
		os.Exit(1)
	}
	// Convert to absolute paths
	projectPath, err := filepath.Abs(trimQuotesAndSpaces(*project_path))
	if err != nil {
		panic(err)
	}
	outputDir, err := filepath.Abs(trimQuotesAndSpaces(*outputDirPath))
	if err != nil {
		panic(err)
	}
	varPrefixTrimmed := trimQuotesAndSpaces(*varPrefix)

	// Initialize the map
	isAliasDuplicate = make(map[string]int)
	wireVars := make(map[string][]wireVar) // Key is wire set name (the part after the prefix) and value is wireVar

	pkg := loadPkgs(projectPath)
	// Step 1: Walk project
	err = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		// Step 2: Parse file
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		// Step 2b: Look for "var ${varPrefix}"
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				val, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range val.Names {
					if strings.HasPrefix(name.Name, varPrefixTrimmed) {
						wireSetName := name.Name[len(varPrefixTrimmed):]
						if wireSetName == "" {
							wireSetName = defaultWireSetName
						}
						// Found one
						rel, _ := filepath.Rel(projectPath, filepath.Dir(path))
						importPath := pkg.PkgPath + "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")
						alias := strings.ReplaceAll(filepath.Base(importPath), "-", "_")
						isAliasDuplicate[alias]++
						if isAliasDuplicate[alias] > 1 {
							alias += fmt.Sprintf("_%d", isAliasDuplicate[alias])
						}

						if _, exists := wireVars[wireSetName]; !exists {
							// Initialize slice
							wireVars[wireSetName] = []wireVar{}
						}
						wireVars[wireSetName] = append(wireVars[wireSetName], wireVar{ImportPath: importPath, Alias: alias})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	// Step 4: Generate file
	// Clean output dir
	_ = os.RemoveAll(outputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		panic(err)
	}
	for wiresetName, setnameWireVars := range wireVars {
		trimedWiresetName := trimNotValidCharForVariable(wiresetName)
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("package %s\n\n", trimQuotesAndSpaces(*genPackageName)))
		buf.WriteString("import (\n")
		buf.WriteString(`  "github.com/google/wire"` + "\n")
		for _, w := range setnameWireVars {
			buf.WriteString(fmt.Sprintf("  %s \"%s\"\n", w.Alias, w.ImportPath))
		}
		buf.WriteString(")\n\n")
		if wiresetName == defaultWireSetName {
			// default wireset is not public variable
			buf.WriteString(fmt.Sprintf("var %sWireSet = wire.NewSet(\n",
				trimedWiresetName))

			buf.WriteString("  // Default WireSet collects all WireSets without a specific name\n")
			for _, w := range setnameWireVars {
				buf.WriteString(fmt.Sprintf("  %s.WireSet,\n", w.Alias))
			}
		} else {
			buf.WriteString(fmt.Sprintf("var %sWireSet = wire.NewSet(\n",
				toCapitalized(trimedWiresetName)))
			buf.WriteString(fmt.Sprintf("  // WireSet for '%s' will include Default WireSet and additional dedicated wire sets\n", wiresetName))
			buf.WriteString(fmt.Sprintf("  %sWireSet,\n", defaultWireSetName))
			for _, w := range setnameWireVars {
				buf.WriteString(fmt.Sprintf("  %s.WireSet%s,\n", w.Alias, wiresetName))
			}
		}

		buf.WriteString(")\n")
		// Format output
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			fmt.Printf("Generated code before formatting:\n%s\n", buf.String())
			panic(err)
		}

		// Write file
		filename := filepath.Join(outputDir,
			strings.ToLower(trimedWiresetName)+".go")
		if err := os.WriteFile(filename, formatted, 0644); err != nil {
			panic(err)
		}
		fmt.Printf("Generated %s with %d WireSets\n", filename, len(setnameWireVars))
	}
}

func loadPkgs(packageFolderPath string) *packages.Package {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
	}
	pkgs, err := packages.Load(cfg, packageFolderPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) != 1 {
		log.Fatalf(
			"%s pattern path find %d pkgs. Give exactly 1 package",
			packageFolderPath, len(pkgs))
	}

	return pkgs[0]
}

func trimQuotesAndSpaces(s string) string {
	return strings.Trim(s, `" `)
}

func trimNotValidCharForVariable(s string) string {
	return strings.Trim(s, `-_./\ "'`)
}

func toCapitalized(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
