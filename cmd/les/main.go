package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"les/pkg/eval"
	"les/pkg/lexer"
	"les/pkg/parser"
	"les/pkg/types"
)

var Version = ""

func getVersion() string {
	if Version != "" {
		return Version
	}
	out, err := exec.Command("git", "rev-list", "--count", "master").Output()
	if err == nil {
		count := strings.TrimSpace(string(out))
		if count != "" {
			return "0.1." + count
		}
	}
	return "0.1.dev"
}

func getPlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func runREPL(syntaxOnly, typeOnly bool) error {
	// Reserved for future interactive shell (REPL) implementation
	return errors.New("REPL is not yet implemented")
}

func runPipeline(codeBytes []byte, filename string, baseDir string, scriptArgs []string, syntaxOnly, typeOnly bool) error {
	// 1. Lexer & Parser
	l := lexer.NewLexer(string(codeBytes))
	p := parser.NewParser(l)
	p.SetFilename(filename)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		for _, parseErr := range p.Errors() {
			fmt.Fprintf(os.Stderr, "Syntax Error: %s\n", parseErr)
		}
		return errors.New("syntax error")
	}

	if syntaxOnly {
		return nil
	}

	// 2. Type inference & checking
	inferencer := types.NewInferencer()
	typeLoader := types.NewFileModuleLoader(baseDir)
	inferencer.SetModuleLoader(typeLoader.Load)

	if err := inferencer.InferProgram(prog); err != nil {
		fmt.Fprintf(os.Stderr, "Type Error: %v\n", err)
		return err
	}

	if typeOnly {
		return nil
	}

	// 3. Evaluator (call-by-need graph reduction runtime)
	evaluator := eval.NewEvaluator()
	evaluator.SetInferencer(inferencer)
	evaluator.SetProgramName(filename)
	evaluator.SetScriptName(filename)
	evaluator.SetArgs(scriptArgs)

	evalLoader := eval.NewFileModuleLoader(baseDir)
	evaluator.SetFileModuleLoader(evalLoader)

	if _, err := evaluator.EvalProgram(prog); err != nil {
		var exitErr *eval.ExitError
		if errors.As(err, &exitErr) {
			return exitErr
		}
		fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
		return err
	}
	return nil
}

func main() {
	showVersion := flag.Bool("v", false, "display version and exit")
	syntaxOnly := flag.Bool("c", false, "check syntax only")
	typeOnly := flag.Bool("t", false, "check syntax and types only")
	inlineCode := flag.String("e", "", "evaluate inline script")

	flag.Usage = func() {
		fmt.Println("Les - Lazy Evaluation Script interpreter")
		fmt.Println("Usage: les [options] [file.les | -] [args...]")
		fmt.Println("       les [options] -e <code> [args...]")
		fmt.Println("Options:")
		fmt.Println("  -v         display version and exit")
		fmt.Println("  -c         check syntax only")
		fmt.Println("  -t         check syntax and types only")
		fmt.Println("  -e <code>  evaluate inline script")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("les %s (%s)\n", getVersion(), getPlatform())
		return
	}

	args := flag.Args()

	var codeBytes []byte
	var filename string
	var baseDir string
	var scriptArgs []string

	if *inlineCode != "" {
		codeBytes = []byte(*inlineCode)
		filename = "-e"
		baseDir = "."
		scriptArgs = args
	} else if len(args) == 0 || args[0] == "-" {
		filename = "-"
		baseDir = "."
		if len(args) > 0 {
			scriptArgs = args[1:]
		} else {
			scriptArgs = []string{}
		}

		if isTerminal(os.Stdin) {
			// Future work: invoke runREPL(*syntaxOnly, *typeOnly)
			// Currently follows Plan A: read from stdin until EOF
		}

		var err error
		codeBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from standard input: %v\n", err)
			os.Exit(1)
		}
	} else {
		filename = args[0]
		scriptArgs = args[1:]
		baseDir = filepath.Dir(filename)

		var err error
		codeBytes, err = os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
	}

	if err := runPipeline(codeBytes, filename, baseDir, scriptArgs, *syntaxOnly, *typeOnly); err != nil {
		var exitErr *eval.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}
