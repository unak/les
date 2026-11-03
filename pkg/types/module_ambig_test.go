package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestEmbeddedStructDirectSelectorAmbiguity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "les_embed_ambig_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m1Code := `module m1
struct Base:
    x: Int
`
	m2Code := `module m2
struct Base:
    y: String
`
	if err := os.WriteFile(filepath.Join(tempDir, "m1.les"), []byte(m1Code), 0644); err != nil {
		t.Fatalf("failed to write m1.les: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "m2.les"), []byte(m2Code), 0644); err != nil {
		t.Fatalf("failed to write m2.les: %v", err)
	}

	// 1. Ambiguous direct selector
	ambigCode := `
import m1
import m2
struct Multi:
    m1::Base
    m2::Base

m = Multi()
v = m.Base
`
	loader := NewFileModuleLoader(tempDir)
	inf := NewInferencer()
	inf.SetModuleLoader(loader.Load)
	prog := parser.NewParser(lexer.NewLexer(ambigCode)).ParseProgram()
	err = inf.InferProgram(prog)
	if err == nil || !strings.Contains(err.Error(), "ambiguous selector 'Base' on type Multi") {
		t.Fatalf("expected ambiguous selector 'Base' error, got: %v", err)
	}

	// 2. Direct field shadows embedded Base
	shadowCode := `
import m1
import m2
struct MultiWithShadow:
    m1::Base
    m2::Base
    Base: Int

m = MultiWithShadow(42)
v = m.Base
`
	inf2 := NewInferencer()
	inf2.SetModuleLoader(loader.Load)
	p2 := parser.NewParser(lexer.NewLexer(shadowCode))
	prog2 := p2.ParseProgram()
	if len(p2.Errors()) > 0 {
		t.Fatalf("parser errors on shadowCode: %v", p2.Errors())
	}
	err = inf2.InferProgram(prog2)
	if err != nil {
		t.Fatalf("expected shadowing to resolve ambiguous selector, got: %v", err)
	}
	vType, ok := inf2.Scope().Lookup("v")
	if !ok || vType != TypeInt {
		t.Fatalf("expected v to have TypeInt, got: %v", vType)
	}
}
