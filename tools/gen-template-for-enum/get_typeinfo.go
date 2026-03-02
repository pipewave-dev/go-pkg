package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"log"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/tools/go/packages"
)

func loadPkgs(packageFolderPath string) *packages.Package {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax,
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

// EnumTypeInfo
type EnumTypeInfo struct {
	Name       string
	Pkg        *packages.Package
	EnumValues []*EnumValues
}

type EnumValues struct {
	Name            string
	Value           string
	Comment         string
	DocComment      []string
	BlockDocComment []string
}

// getType
func getType(pkg *packages.Package, typeName string) *EnumTypeInfo {
	// Define result variable
	r := &EnumTypeInfo{
		Name: typeName,
		Pkg:  pkg,
	}

	// Traverse packages and Go files in the package
	for _, file := range pkg.Syntax {
		ast.Inspect(file, r.genDecl)
	}

	if len(r.EnumValues) == 0 {
		log.Fatalf("No value for enum type %s", typeName)
	}

	return r
}

func (i *EnumTypeInfo) genDecl(node ast.Node) bool {
	decl, ok := node.(*ast.GenDecl)
	if !ok {
		return true
	}
	switch decl.Tok {
	case token.CONST:
		// if name = i.typeName
		i.resolveEnumValues(decl)
		return false
	default:
		return true
	}
}

// func (i *EnumTypeInfo) resolveEnumType(decl *ast.GenDecl) {
// 	if decl.Tok != token.TYPE {
// 		log.Fatal("pass wrong decl. This func required a token.TYPE")
// 	}

// 	for _, spec := range decl.Specs {
// 		tSpec, _ := spec.(*ast.TypeSpec) // Guaranteed to succeed as this is TYPE.
// 		if tSpec.Name.Name == i.Name {
// 			i.Type = tSpec
// 		}
// 	}
// }

func (i *EnumTypeInfo) resolveEnumValues(decl *ast.GenDecl) {
	if decl.Tok != token.CONST {
		log.Fatal("pass wrong decl. This func required a token.CONST")
	}

	// Block Doc comment
	var (
		astBlockDocComment []*ast.Comment
		blockDocComment    []string
	)

	lo.Try0(func() {
		astBlockDocComment = decl.Doc.List
	})
	if len(astBlockDocComment) > 0 {
		blockDocComment = lo.Map(decl.Doc.List, func(c *ast.Comment, i int) string {
			if c != nil {
				return strings.Trim(c.Text, "/ ")
			}
			return ""
		})
	}

	result := make([]*EnumValues, 0)

	typ := "" // when enum is iota, type is reused the above ident
	for _, spec := range decl.Specs {
		vSpec := spec.(*ast.ValueSpec) // Guaranteed to succeed as this is CONST.

		//
		// Doc comment
		var (
			astDocComment []*ast.Comment
			docComment    []string
		)
		lo.Try0(func() {
			astDocComment = vSpec.Doc.List
		})
		if len(astDocComment) > 0 {
			docComment = lo.Map(vSpec.Doc.List, func(c *ast.Comment, i int) string {
				if c != nil {
					return strings.Trim(c.Text, "/ ")
				}
				return ""
			})
		}

		//
		// Comment
		var (
			astComment *ast.Comment
			comment    string
		)
		lo.Try0(func() {
			astComment = vSpec.Comment.List[0]
		})
		if astComment != nil {
			comment = strings.Trim(astComment.Text, "/ ")
		}

		// Find typ for Spec
		if vSpec.Type == nil && len(vSpec.Values) > 0 {
			// iota only
			// "X = 1". With no type but a value. If the constant is untyped,
			// find type by call Expr
			typ = ""
			ce, ok := vSpec.Values[0].(*ast.CallExpr)
			if !ok {
				continue // unexpected
			}
			id, ok := ce.Fun.(*ast.Ident)
			if !ok {
				continue // unexpected
			}
			typ = id.Name
		}
		if vSpec.Type != nil {
			switch vSpecTy := vSpec.Type.(type) {
			case *ast.Ident:
				typ = vSpecTy.Name
			case *ast.SelectorExpr:
				typ = vSpecTy.X.(*ast.Ident).Name + "." + vSpecTy.Sel.Name
			default:
				continue
			}
		}
		if typ != i.Name {
			// This is not the type we're looking for.
			continue
		}

		//
		for _, name := range vSpec.Names {
			if name.Name == "_" {
				continue // ignore `_`
			}
			constObj, ok := i.Pkg.TypesInfo.Defs[name]
			if !ok {
				log.Fatalf("no value for constant %s", name)
			}
			// get value of const
			valueStr := getValueConstObj(constObj)
			// get comment

			v := &EnumValues{
				Name:            name.Name,
				Value:           valueStr,
				Comment:         comment,
				DocComment:      docComment,
				BlockDocComment: blockDocComment,
			}
			if c := vSpec.Comment; c != nil && len(c.List) == 1 {
				v.Comment = strings.TrimSpace(c.Text())
			}
			result = append(result, v)
		}
	}

	if len(result) > 0 {
		i.EnumValues = append(i.EnumValues, result...)
	}
}

func getValueConstObj(constObj types.Object) string {
	var valueStr string
	info := constObj.Type().Underlying().(*types.Basic).Info()
	switch {
	case info&types.IsString != 0:
		// get Val
		value := constObj.(*types.Const).Val() // Guaranteed to succeed as this is CONST.
		if value.Kind() != constant.String {
			log.Fatalf("can't happen: constant is not a string") // unexpected
		}
		valueStr = constant.StringVal(value)
	case info&types.IsInteger != 0:
		value := constObj.(*types.Const).Val() // Guaranteed to succeed as this is CONST.
		if value.Kind() != constant.Int {
			log.Fatalf("can't happen: constant is not a string") // unexpected
		}
		valInt, _ := constant.Int64Val(value)
		valueStr = strconv.FormatInt(valInt, 10)
	}
	return valueStr

}
