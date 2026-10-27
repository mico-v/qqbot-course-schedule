// Command check-ports enforces the context-first convention for Go ports.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

func main() {
	files := []string{
		filepath.Join("internal", "schedule", "ports.go"),
		filepath.Join("internal", "schedule", "panelstore.go"),
	}
	failed := false
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: parse %s: %v\n", path, err)
			os.Exit(1)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			interfaceType, ok := node.(*ast.InterfaceType)
			if !ok || interfaceType.Methods == nil {
				return true
			}
			for _, field := range interfaceType.Methods.List {
				function, ok := field.Type.(*ast.FuncType)
				if !ok || function.Params == nil || len(function.Params.List) == 0 {
					continue
				}
				selector, ok := function.Params.List[0].Type.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Context" {
					position := fset.Position(field.Pos())
					fmt.Fprintf(
						os.Stderr,
						"ERROR: %s:%d port %s must take context.Context first\n",
						position.Filename,
						position.Line,
						field.Names[0].Name,
					)
					failed = true
				}
			}
			return true
		})
	}
	if failed {
		os.Exit(1)
	}
}
