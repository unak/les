package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"les/pkg/eval"
	"les/pkg/lexer"
	"les/pkg/parser"
	"les/pkg/types"
)

func TestExamples(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("failed to read examples directory: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".les") {
			continue
		}
		count++
		fileName := entry.Name()
		filePath := filepath.Join(examplesDir, fileName)

		t.Run(fileName, func(t *testing.T) {
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("failed to read file %s: %v", fileName, err)
			}

			// 1. Syntax check
			l := lexer.NewLexer(string(content))
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("syntax errors in %s: %v", fileName, p.Errors())
			}

			// 2. Type check
			inf := types.NewInferencer()
			tLoader := types.NewFileModuleLoader(examplesDir)
			inf.SetModuleLoader(tLoader.Load)
			if err := inf.InferProgram(prog); err != nil {
				t.Fatalf("type error in %s: %v", fileName, err)
			}

			// 3. Execution check (re-parse for fresh AST nodes)
			lExec := lexer.NewLexer(string(content))
			pExec := parser.NewParser(lExec)
			progExec := pExec.ParseProgram()

			ev := eval.NewEvaluator()
			ev.SetProgramName(filePath)
			ev.SetScriptName(filePath)
			ev.SetArgs([]string{"arg1", "arg2"})
			ev.SetInput(strings.NewReader("sample line\n"))
			var out strings.Builder
			ev.SetOutput(&out)
			eLoader := eval.NewFileModuleLoader(examplesDir)
			ev.SetFileModuleLoader(eLoader)

			if _, err := ev.EvalProgram(progExec); err != nil {
				t.Fatalf("runtime error in %s: %v", fileName, err)
			}

			if fileName == "quine.les" {
				if out.String() != string(content) {
					t.Fatalf("quine output mismatch:\nExpected (%d bytes):\n%s\nGot (%d bytes):\n%s", len(content), string(content), len(out.String()), out.String())
				}
			}
		})
	}

	if count == 0 {
		t.Fatalf("no .les files found in %s", examplesDir)
	}
}

func TestCLIInlineOneLiner(t *testing.T) {
	code := `x: Int = 10; y: Int = 20; print x + y, "\n"`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("syntax errors: %v", p.Errors())
	}

	inf := types.NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("type error: %v", err)
	}

	lExec := lexer.NewLexer(code)
	pExec := parser.NewParser(lExec)
	progExec := pExec.ParseProgram()

	ev := eval.NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	if _, err := ev.EvalProgram(progExec); err != nil {
		t.Fatalf("runtime error: %v", err)
	}

	if strings.TrimSpace(out.String()) != "30" {
		t.Fatalf("expected output '30', got %q", out.String())
	}
}

func TestVersionAndPlatform(t *testing.T) {
	v := getVersion()
	if !strings.HasPrefix(v, "0.1.") {
		t.Fatalf("expected version starting with '0.1.', got %q", v)
	}

	p := getPlatform()
	expectedPlat := runtime.GOOS + "/" + runtime.GOARCH
	if p != expectedPlat {
		t.Fatalf("expected platform %q, got %q", expectedPlat, p)
	}
}

func TestIsTerminal(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "terminal_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if isTerminal(tmpFile) {
		t.Fatalf("expected regular file to not be a terminal")
	}
}

func TestRunPipelineStdin(t *testing.T) {
	// 1. Success execution
	code := []byte("x: Int = 10; y: Int = 20; print x + y\n")
	err := runPipeline(code, "-", ".", []string{}, false, false)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// 2. Syntax only (-c)
	err = runPipeline(code, "-", ".", []string{}, true, false)
	if err != nil {
		t.Fatalf("expected success with syntaxOnly, got %v", err)
	}

	// 3. Type only (-t)
	err = runPipeline(code, "-", ".", []string{}, false, true)
	if err != nil {
		t.Fatalf("expected success with typeOnly, got %v", err)
	}

	// 4. Syntax error
	badSyntax := []byte("def (x)\n")
	err = runPipeline(badSyntax, "-", ".", []string{}, false, false)
	if err == nil {
		t.Fatalf("expected syntax error, got nil")
	}

	// 5. Type error
	badType := []byte("x: Int = 'abc'\n")
	err = runPipeline(badType, "-", ".", []string{}, false, false)
	if err == nil {
		t.Fatalf("expected type error, got nil")
	}
}

func TestRunREPLPlaceholder(t *testing.T) {
	err := runREPL(false, false)
	if err == nil || !strings.Contains(err.Error(), "REPL is not yet implemented") {
		t.Fatalf("expected REPL not yet implemented error, got %v", err)
	}
}

func TestErrorReportingFormat(t *testing.T) {
	// Syntax error
	l := lexer.NewLexer("x = 10 + ;")
	p := parser.NewParser(l)
	p.SetFilename("test.les")
	p.ParseProgram()
	if len(p.Errors()) == 0 {
		t.Fatalf("expected syntax errors")
	}
	expectedSyntax := "test.les:1: unexpected token ';' at col 10"
	if p.Errors()[0] != expectedSyntax {
		t.Fatalf("expected syntax error %q, got %q", expectedSyntax, p.Errors()[0])
	}

	// Type error
	lType := lexer.NewLexer("x: Int = \"str\";")
	pType := parser.NewParser(lType)
	pType.SetFilename("type_test.les")
	progType := pType.ParseProgram()
	inf := types.NewInferencer()
	typeErr := inf.InferProgram(progType)
	if typeErr == nil {
		t.Fatalf("expected type error")
	}
	expectedType := "type_test.les:1: type mismatch for variable 'x': expected Int, got String at col 1"
	if typeErr.Error() != expectedType {
		t.Fatalf("expected type error %q, got %q", expectedType, typeErr.Error())
	}

	// Runtime error
	lRun := lexer.NewLexer("x = 10\nprint((x)[0])")
	pRun := parser.NewParser(lRun)
	pRun.SetFilename("run_test.les")
	progRun := pRun.ParseProgram()
	ev := eval.NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	_, runErr := ev.EvalProgram(progRun)
	if runErr == nil {
		t.Fatalf("expected runtime error")
	}
	expectedRun := "run_test.les:2: cannot index non-tuple/non-string/non-bytes/non-dict value: 10 at col 10"
	if runErr.Error() != expectedRun {
		t.Fatalf("expected runtime error %q, got %q", expectedRun, runErr.Error())
	}

	// Type Warning
	lWarn := lexer.NewLexer("def foo(x):\n  return x\n")
	pWarn := parser.NewParser(lWarn)
	pWarn.SetFilename("warn_test.les")
	progWarn := pWarn.ParseProgram()
	infWarn := types.NewInferencer()
	_ = infWarn.InferProgram(progWarn)
	if len(infWarn.Warnings()) == 0 {
		t.Fatalf("expected type warnings")
	}
	expectedWarn := "warn_test.les:1: Unknown type of parameter 'x' in function 'foo' at col 9"
	if infWarn.Warnings()[0].String() != expectedWarn {
		t.Fatalf("expected warning %q, got %q", expectedWarn, infWarn.Warnings()[0].String())
	}
}


