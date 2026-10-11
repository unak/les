package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"les/pkg/eval"
	"les/pkg/lexer"
	"les/pkg/parser"
	"les/pkg/types"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		fmt.Println("Les - Lazy Evaluation Script interpreter")
		fmt.Println("Usage: les <file.les>")
		os.Exit(1)
	}

	filename := args[0]
	codeBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// 1. Lexer
	l := lexer.NewLexer(string(codeBytes))

	// 2. Parser
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		for _, parseErr := range p.Errors() {
			fmt.Fprintf(os.Stderr, "Syntax Error: %s\n", parseErr)
		}
		os.Exit(1)
	}

	// 3. Type inference & checking
	inferencer := types.NewInferencer()
	if err := inferencer.InferProgram(prog); err != nil {
		fmt.Fprintf(os.Stderr, "Type Error: %v\n", err)
		os.Exit(1)
	}

	// 4. Evaluator (call-by-need graph reduction runtime)
	evaluator := eval.NewEvaluator()
	evaluator.SetProgramName(filename)
	evaluator.SetScriptName(filename)
	evaluator.SetArgs(args[1:])

	baseDir := filepath.Dir(filename)
	loader := eval.NewFileModuleLoader(baseDir)
	evaluator.SetModuleLoader(loader.Load)

	if _, err := evaluator.EvalProgram(prog); err != nil {
		var exitErr *eval.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
		os.Exit(1)
	}
}
