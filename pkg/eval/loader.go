package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"les/pkg/lexer"
	"les/pkg/parser"
)

// FileModuleLoader loads and executes other .les files as modules.
type FileModuleLoader struct {
	baseDir   string
	parentEnv *Environment
	cache     map[string]*ModuleValue
	loading   map[string]bool // circular import detection
}

func NewFileModuleLoader(baseDir string) *FileModuleLoader {
	return &FileModuleLoader{
		baseDir: baseDir,
		cache:   make(map[string]*ModuleValue),
		loading: make(map[string]bool),
	}
}

func (fml *FileModuleLoader) SetParentEnv(env *Environment) {
	fml.parentEnv = env
}

// Load loads the module from path (e.g. "math" -> baseDir/math.les or baseDir/math/module.les).
func (fml *FileModuleLoader) Load(modPath string) (*ModuleValue, error) {
	if mod, ok := fml.cache[modPath]; ok {
		return mod, nil
	}

	if fml.loading[modPath] {
		return nil, fmt.Errorf("circular import detected for module '%s'", modPath)
	}
	fml.loading[modPath] = true
	defer func() {
		delete(fml.loading, modPath)
	}()

	// Resolve file path
	targetFile := filepath.Join(fml.baseDir, modPath+".les")
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		// Try directory module: modPath/main.les or modPath/module.les
		targetFile = filepath.Join(fml.baseDir, modPath, "module.les")
		if _, err2 := os.Stat(targetFile); os.IsNotExist(err2) {
			targetFile = filepath.Join(fml.baseDir, modPath, "main.les")
			if _, err3 := os.Stat(targetFile); os.IsNotExist(err3) {
				return nil, fmt.Errorf("module file not found: '%s.les' or '%s/module.les'", modPath, modPath)
			}
		}
	}

	contentBytes, err := os.ReadFile(targetFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read module file %s: %w", targetFile, err)
	}

	// Lex & Parse
	l := lexer.NewLexer(string(contentBytes))
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("syntax error in module '%s': %s", modPath, strings.Join(p.Errors(), "; "))
	}

	// Module name: explicit declaration or path basename
	modName := prog.ModuleName
	if modName == "" {
		modName = filepath.Base(modPath)
	}

	// Execute module in dedicated isolated environment chained to parentEnv
	modEnv := NewEnvironment(fml.parentEnv)
	modEnv.Loader = fml.Load
	modEnv.Set("scriptName", StringValue(targetFile))

	for _, stmt := range prog.Body {
		if _, err := modEnv.evalStmt(stmt); err != nil {
			return nil, fmt.Errorf("runtime error in module '%s': %w", modName, err)
		}
	}

	// Collect exported members: uppercase identifiers only
	exports := make(map[string]Value)
	for name, val := range modEnv.store {
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			if idv, ok := val.(*InterfaceDefValue); ok && idv.ModuleName == "" {
				idv.ModuleName = modName
			}
			exports[name] = val
		}
	}

	modVal := &ModuleValue{
		Name:    modName,
		Exports: exports,
	}

	fml.cache[modPath] = modVal
	return modVal, nil
}
