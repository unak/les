package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"les/pkg/ast"
	"les/pkg/lexer"
	"les/pkg/parser"
)

// TestFinding1ModuleLoaderCacheImmutability verifies that importing a module with an alias
// does not mutate the cached ModuleValue in FileModuleLoader.
func TestFinding1ModuleLoaderCacheImmutability(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "les_mod_cache_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	modCode := `module mymod

def Greet():
    "hello"
`
	if err := os.WriteFile(filepath.Join(tempDir, "mymod.les"), []byte(modCode), 0644); err != nil {
		t.Fatalf("failed to write mymod.les: %v", err)
	}

	loader := NewFileModuleLoader(tempDir)

	// First evaluation: import with alias
	ev1 := NewEvaluator()
	ev1.SetFileModuleLoader(loader)
	script1 := `import "mymod" as m1
res = m1::Greet()
`
	prog1 := parser.NewParser(lexer.NewLexer(script1)).ParseProgram()
	_, err = ev1.EvalProgram(prog1)
	if err != nil {
		t.Fatalf("eval script1 failed: %v", err)
	}
	v1, ok := ev1.globalEnv.Get("res")
	if !ok || Force(v1).String() != "hello" {
		t.Fatalf("expected 'hello', got %v", v1)
	}

	// Verify the cached module in loader retains its canonical name "mymod"
	cachedMod, ok := loader.cache["mymod"]
	if !ok {
		t.Fatalf("expected 'mymod' in loader cache")
	}
	if cachedMod.Name != "mymod" {
		t.Fatalf("cached module Name was mutated to '%s', expected 'mymod'", cachedMod.Name)
	}

	// Second evaluation: import without alias
	ev2 := NewEvaluator()
	ev2.SetFileModuleLoader(loader)
	script2 := `import "mymod"
res = mymod::Greet()
`
	prog2 := parser.NewParser(lexer.NewLexer(script2)).ParseProgram()
	_, err = ev2.EvalProgram(prog2)
	if err != nil {
		t.Fatalf("eval script2 failed: %v", err)
	}
	v2, ok := ev2.globalEnv.Get("res")
	if !ok || Force(v2).String() != "hello" {
		t.Fatalf("expected 'hello', got %v", v2)
	}
}

// TestFinding3SpecializeGenericValueBuiltinFunc verifies that specializeGenericValue
// properly unwraps and specializes BuiltinFunc.TypeDef.
func TestFinding3SpecializeGenericValueBuiltinFunc(t *testing.T) {
	env := NewEnvironment(nil)
	sDef := &StructDefValue{
		Name: "Box",
		TypeParams: []ast.TypeParam{
			{Name: "T"},
		},
		FieldOrder: []string{"val"},
	}
	bf := &BuiltinFunc{
		Name:    "Box",
		TypeDef: sDef,
	}

	specialized, err := specializeGenericValue(bf, []string{"Int"}, env)
	if err != nil {
		t.Fatalf("specializeGenericValue failed: %v", err)
	}
	specBf, ok := specialized.(*BuiltinFunc)
	if !ok {
		t.Fatalf("expected *BuiltinFunc, got %T", specialized)
	}
	specDef, ok := specBf.TypeDef.(*StructDefValue)
	if !ok {
		t.Fatalf("expected *StructDefValue, got %T", specBf.TypeDef)
	}
	if len(specDef.TypeArgs) != 1 || specDef.TypeArgs[0] != "Int" {
		t.Fatalf("expected TypeArgs ['Int'], got %v", specDef.TypeArgs)
	}
}

// TestFinding5RuntimeEmbeddedStructAmbiguity verifies runtime ambiguous selector error
// when struct embeds multiple structs with colliding member names, and resolution via shadowing.
func TestFinding5RuntimeEmbeddedStructAmbiguity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "les_rt_ambig_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m1Code := `module m1
struct Base:
    val: Int = 10
`
	m2Code := `module m2
struct Base:
    val: Int = 20
`
	if err := os.WriteFile(filepath.Join(tempDir, "m1.les"), []byte(m1Code), 0644); err != nil {
		t.Fatalf("failed to write m1.les: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "m2.les"), []byte(m2Code), 0644); err != nil {
		t.Fatalf("failed to write m2.les: %v", err)
	}

	loader := NewFileModuleLoader(tempDir)

	// 1. Ambiguous selector
	ev1 := NewEvaluator()
	ev1.SetFileModuleLoader(loader)
	script1 := `import m1
import m2
struct Multi:
    m1::Base
    m2::Base

m = Multi()
print m.Base
`
	prog1 := parser.NewParser(lexer.NewLexer(script1)).ParseProgram()
	_, err = ev1.EvalProgram(prog1)
	if err == nil || !strings.Contains(err.Error(), "ambiguous selector 'Base' on type Multi") {
		t.Fatalf("expected ambiguous selector 'Base' error, got: %v", err)
	}

	// 2. Direct field shadows embedded Base
	ev2 := NewEvaluator()
	ev2.SetFileModuleLoader(loader)
	script2 := `import m1
import m2
struct MultiWithShadow:
    m1::Base
    m2::Base
    Base: Int = 99

m = MultiWithShadow(Base = 42)
res = m.Base
`
	prog2 := parser.NewParser(lexer.NewLexer(script2)).ParseProgram()
	_, err = ev2.EvalProgram(prog2)
	if err != nil {
		t.Fatalf("expected shadowing to resolve ambiguous selector, got: %v", err)
	}
	v2, ok := ev2.globalEnv.Get("res")
	if !ok || Force(v2).String() != "42" {
		t.Fatalf("expected 42, got: %v", v2)
	}
}
