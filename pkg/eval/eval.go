package eval

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"les/pkg/ast"
	"les/pkg/types"
)

var callIDCounter uint64

func nextCallID() uint64 {
	return atomic.AddUint64(&callIDCounter, 1)
}

// Evaluator runs Les AST nodes.
type Evaluator struct {
	builtinsEnv      *Environment
	globalEnv        *Environment
	inferencer       *types.Inferencer
	fileModuleLoader *FileModuleLoader
}

func NewEvaluator() *Evaluator {
	bEnv := NewEnvironment(nil)
	ev := &Evaluator{builtinsEnv: bEnv, globalEnv: NewEnvironment(bEnv)}
	ev.initIOModule()
	ev.initBufioModule()
	ev.registerBuiltins()
	return ev
}

func (ev *Evaluator) SetInferencer(inf *types.Inferencer) {
	ev.inferencer = inf
	if ev.builtinsEnv != nil {
		ev.builtinsEnv.Inferencer = inf
	}
	if ev.globalEnv != nil {
		ev.globalEnv.Inferencer = inf
	}
}

func (ev *Evaluator) SetExprTypes(exprTypes map[ast.Expr]types.Type) {
	if ev.inferencer == nil {
		ev.inferencer = types.NewInferencer()
	}
	ev.inferencer.ExprTypes = exprTypes
	if ev.builtinsEnv != nil {
		ev.builtinsEnv.Inferencer = ev.inferencer
	}
	if ev.globalEnv != nil {
		ev.globalEnv.Inferencer = ev.inferencer
	}
}

func (ev *Evaluator) newThunk(expr ast.Expr, env *Environment) *Thunk {
	thunk := NewThunk(expr, env)
	if expr != nil && ev != nil && ev.inferencer != nil && ev.inferencer.ExprTypes != nil {
		if t, ok := ev.inferencer.ExprTypes[expr]; ok {
			thunk.InferredType = t
		}
	}
	return thunk
}

func (ev *Evaluator) SetOutput(w io.Writer) {
	if oldBW := ev.builtinsEnv.StdoutWriter; oldBW != nil {
		_ = oldBW.Flush()
		oldBW.Reset(io.Discard)
		ev.builtinsEnv.UnregisterBufioWriter(oldBW)
	}
	ev.builtinsEnv.RawStdout = w
	if w != nil {
		bw := bufio.NewWriter(w)
		ev.builtinsEnv.StdoutWriter = bw
		ev.builtinsEnv.RegisterBufioWriter(bw, w)
	} else {
		ev.builtinsEnv.StdoutWriter = nil
	}
}

func (ev *Evaluator) SetErrorOutput(w io.Writer) {
	ev.builtinsEnv.RawStderr = w
}

func (ev *Evaluator) SetInput(r io.Reader) {
	ev.builtinsEnv.RawStdin = r
	if r != nil {
		ev.builtinsEnv.StdinReader = bufio.NewReader(r)
	} else {
		ev.builtinsEnv.StdinReader = nil
	}
}

func (ev *Evaluator) callValue(calleeVal Value, args []Value) (Value, error) {
	forced := Force(calleeVal)
	switch fn := forced.(type) {
	case *BuiltinFunc:
		return fn.Fn(args)
	case *Closure:
		return ev.globalEnv.invokeClosure(fn, args)
	default:
		return nil, fmt.Errorf("cannot call non-function value: %v", forced)
	}
}

func (ev *Evaluator) SetModuleLoader(loader func(path string) (*ModuleValue, error)) {
	ev.builtinsEnv.Loader = loader
	ev.globalEnv.Loader = loader
}

func (ev *Evaluator) SetFileModuleLoader(fml *FileModuleLoader) {
	ev.fileModuleLoader = fml
	if fml != nil {
		fml.SetParentEnv(ev.builtinsEnv)
		ev.builtinsEnv.Loader = fml.Load
		ev.globalEnv.Loader = fml.Load
	}
}

func (ev *Evaluator) registerBuiltins() {
	// print: built-in effect boundary. Demands evaluation of its arguments without automatic newline.
	ev.builtinsEnv.Set("print", &BuiltinFunc{
		Name:     "print",
		FuncType: types.BuiltinPrintFuncType,
		Fn: func(args []Value) (Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				val := Force(a)
				if !checkValueMatchesType(val, "Show", ev.globalEnv) {
					return nil, fmt.Errorf("type error in print: argument %d of type %s does not implement Show interface", i+1, typeNameOf(val))
				}
				parts[i] = val.String()
			}
			out := strings.Join(parts, "")
			w := ev.globalEnv.GetStdoutWriter()
			if _, err := w.WriteString(out); err != nil {
				return nil, err
			}
			if err := w.Flush(); err != nil {
				return nil, err
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	})

	// exit: terminates process with exit code (0 or 1 arg, Bool or Int).
	ev.builtinsEnv.Set("exit", &BuiltinFunc{
		Name:     "exit",
		FuncType: types.BuiltinExitFuncType,
		Fn: func(args []Value) (Value, error) {
			code := 0
			if len(args) == 0 {
				code = 0
			} else if len(args) == 1 {
				val := Force(args[0])
				switch v := val.(type) {
				case IntValue:
					if c, ok := v.ToInt(); ok {
						code = c
					} else {
						// Preserve non-zero exit status for out-of-range integers
						low := int(v.ToBig().Int64())
						if low != 0 {
							code = low
						} else {
							code = 1
						}
					}
				case BoolValue:
					if bool(v) {
						code = 0
					} else {
						code = 1
					}
				default:
					return nil, fmt.Errorf("exit expects Int or Bool, got %v", val)
				}
			} else {
				return nil, fmt.Errorf("exit expects 0 or 1 argument, got %d", len(args))
			}
			panic(&ExitError{Code: code})
		},
	})

	// Bytes: creates Bytes from String or empty if 0 args
	ev.builtinsEnv.Set("Bytes", &BuiltinFunc{
		Name:     "Bytes",
		FuncType: types.BuiltinBytesFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) == 0 {
				return BytesValue(""), nil
			}
			if len(args) == 1 {
				forced := Force(args[0])
				switch v := forced.(type) {
				case StringValue:
					return BytesValue(string(v)), nil
				case BytesValue:
					return v, nil
				default:
					return nil, fmt.Errorf("Bytes constructor expects String or Bytes, got %s", typeNameOf(forced))
				}
			}
			return nil, fmt.Errorf("Bytes expects 0 or 1 arguments, got %d", len(args))
		},
	})

	// Global standard I/O streams
	bufioMod, _ := ev.builtinsEnv.GetModule("bufio")
	ioMod, _ := ev.builtinsEnv.GetModule("io")

	stdinConstructor := bufioMod.Exports["Reader"].(*BuiltinFunc)
	stdoutConstructor := bufioMod.Exports["Writer"].(*BuiltinFunc)

	stdinInst, _ := stdinConstructor.Fn([]Value{ioMod.Exports["Stdin"]})
	stdoutInst, _ := stdoutConstructor.Fn([]Value{ioMod.Exports["Stdout"]})
	stderrInst := ioMod.Exports["Stderr"]

	ev.builtinsEnv.Set("Stdin", stdinInst)
	ev.builtinsEnv.Set("Stdout", stdoutInst)
	ev.builtinsEnv.Set("Stderr", stderrInst)

	// gets: reads a single line from standard input, returning String or Nil on EOF.
	// Returns a lazy ActionThunk that only blocks when demanded.
	ev.builtinsEnv.Set("gets", &BuiltinFunc{
		Name:     "gets",
		FuncType: types.BuiltinGetsFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("gets expects 0 arguments, got %d", len(args))
			}
			si := stdinInst.(*StructInstance)
			rlFn := si.Fields["ReadLine"].(*BuiltinFunc)
			return rlFn.Fn([]Value{})
		},
	})

	// Default CLI arguments & program name
	ev.builtinsEnv.Set("Args", &TupleValue{Elements: []Value{}})
	ev.builtinsEnv.Set("ProgramName", StringValue(""))
	ev.builtinsEnv.Set("scriptName", StringValue(""))

	// Built-in Error constructor & default Error struct definition
	errorMethodFuncType := &types.FuncType{
		Arg: &types.TupleType{Elems: []types.Type{}},
		Ret: types.TypeString,
	}
	errorStructDef := &StructDefValue{
		Name:          "SimpleError",
		FieldOrder:    []string{"message"},
		FieldDefaults: make(map[string]ast.Expr),
		EmbeddedNames: []string{},
		Methods: map[string]*ast.FuncDef{
			"Message": {
				Name: "Message",
				Body: []ast.Stmt{
					&ast.ReturnStmt{Value: &ast.Identifier{Name: "message"}},
				},
			},
			"Inspect": {
				Name: "Inspect",
				Body: []ast.Stmt{
					&ast.ReturnStmt{
						Value: &ast.BinaryExpr{
							Op:    "+",
							Left:  &ast.StringLit{Value: "Error: "},
							Right: &ast.Identifier{Name: "message"},
						},
					},
				},
			},
			"Error": {
				Name: "Error",
				Body: []ast.Stmt{
					&ast.ReturnStmt{Value: &ast.Identifier{Name: "message"}},
				},
			},
		},
		Env: ev.builtinsEnv,
		StructType: &types.StructType{
			Name: "SimpleError",
			Fields: map[string]types.Type{
				"message": types.TypeString,
			},
			FieldOrder: []string{"message"},
			Methods: map[string]*types.FuncType{
				"Message": errorMethodFuncType,
				"Inspect": errorMethodFuncType,
				"Error":   errorMethodFuncType,
			},
		},
	}

	ev.builtinsEnv.Set("Error", &BuiltinFunc{
		Name: "Error",
		FuncType: &types.FuncType{
			Arg:      &types.TupleType{Elems: []types.Type{types.BuiltinShowIface}},
			MinArity: 0,
			MaxArity: 1,
			Ret:      types.BuiltinErrorIface,
		},
		Fn: func(args []Value) (Value, error) {
			msg := "error"
			if len(args) > 0 {
				msg = Force(args[0]).String()
			}
			fields := map[string]Value{
				"message": StringValue(msg),
			}
			inst := &StructInstance{
				Def:      errorStructDef,
				Fields:   fields,
				Embedded: make(map[string]*StructInstance),
			}
			return inst, nil
		},
	})

	// Built-in Show interface definition: Inspect(): String, Type(): String
	ev.builtinsEnv.Set("Show", &InterfaceDefValue{
		Name: "Show",
		Methods: map[string]ast.MethodSig{
			"Inspect": {
				Name:   "Inspect",
				Params: []ast.FuncParam{},
			},
			"Type": {
				Name:   "Type",
				Params: []ast.FuncParam{},
			},
		},
	})

	// Built-in Equal interface definition: ==(other: T): Bool
	ev.builtinsEnv.Set("Equal", &InterfaceDefValue{
		Name: "Equal",
		TypeParams: []ast.TypeParam{
			{Name: "T"},
		},
		Methods: map[string]ast.MethodSig{
			"==": {
				Name: "==",
				Params: []ast.FuncParam{
					{Name: "other", TypeAnnot: "T"},
				},
				ReturnType: "Bool",
			},
		},
	})

	// Built-in Comparable interface definition: ==(other: T): Bool, <(other: T): Bool
	ev.builtinsEnv.Set("Comparable", &InterfaceDefValue{
		Name: "Comparable",
		TypeParams: []ast.TypeParam{
			{Name: "T"},
		},
		Embedded: []string{"Equal[T]"},
		Methods: map[string]ast.MethodSig{
			"==": {
				Name: "==",
				Params: []ast.FuncParam{
					{Name: "other", TypeAnnot: "T"},
				},
				ReturnType: "Bool",
			},
			"<": {
				Name: "<",
				Params: []ast.FuncParam{
					{Name: "other", TypeAnnot: "T"},
				},
				ReturnType: "Bool",
			},
		},
	})

	// Built-in Indexable interface definition: [](key: K): V, Len(): Int
	ev.builtinsEnv.Set("Indexable", &InterfaceDefValue{
		Name: "Indexable",
		TypeParams: []ast.TypeParam{
			{Name: "K"},
			{Name: "V"},
		},
		Methods: map[string]ast.MethodSig{
			"[]": {
				Name: "[]",
				Params: []ast.FuncParam{
					{Name: "key", TypeAnnot: "K"},
				},
				ReturnType: "V",
			},
			"Len": {
				Name:       "Len",
				Params:     []ast.FuncParam{},
				ReturnType: "Int",
			},
		},
	})
}

func (ev *Evaluator) SetArgs(args []string) {
	elems := make([]Value, len(args))
	for i, a := range args {
		elems[i] = StringValue(a)
	}
	ev.builtinsEnv.Set("Args", &TupleValue{Elements: elems})
}

func (ev *Evaluator) SetProgramName(name string) {
	ev.builtinsEnv.Set("ProgramName", StringValue(name))
	if ev.builtinsEnv.Filename == "" {
		ev.builtinsEnv.Filename = name
	}
}

func (ev *Evaluator) SetScriptName(name string) {
	ev.globalEnv.Set("scriptName", StringValue(name))
	if ev.globalEnv.Filename == "" {
		ev.globalEnv.Filename = name
	}
}

// ExitError indicates that exit() was called to terminate execution.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit with code %d", e.Code)
}

func typeNameOf(val Value) string {
	return typeNameWithDepth(val, 0)
}

func valueOrThunkTypeNameWithDepth(val Value, depth int) string {
	if evaluated, ok := PeekEvaluated(val); ok {
		return typeNameWithDepth(evaluated, depth)
	}
	if thunk, ok := val.(*Thunk); ok {
		if thunk.InferredType != nil {
			if t, ok := thunk.InferredType.(types.Type); ok && t != types.TypeUnknown {
				return t.String()
			}
		}
		return "Unknown"
	}
	return typeNameWithDepth(Force(val), depth)
}

func joinSortedTypeSet(typeSet map[string]bool) string {
	typesList := make([]string, 0, len(typeSet))
	for t := range typeSet {
		typesList = append(typesList, t)
	}
	sort.Strings(typesList)
	return strings.Join(typesList, " | ")
}

func typeNameWithDepth(val Value, depth int) string {
	if depth > 10 {
		return "..."
	}
	switch v := val.(type) {
	case IntValue:
		return "Int"
	case FloatValue:
		return "Float"
	case StringValue:
		return "String"
	case BytesValue:
		return "Bytes"
	case BoolValue:
		return "Bool"
	case NilValue:
		return "Nil"
	case *TupleValue:
		if len(v.Elements) == 0 {
			if v.IsVariadic {
				return "(...)"
			}
			return "()"
		}
		parts := make([]string, len(v.Elements))
		for i, el := range v.Elements {
			parts[i] = valueOrThunkTypeNameWithDepth(el, depth+1)
		}
		if v.IsVariadic {
			return "(" + strings.Join(parts, ", ") + ", ...)"
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *DictValue:
		if len(v.Entries) == 0 {
			return "{Unknown: Unknown}"
		}
		keyTypesSet := make(map[string]bool)
		valTypesSet := make(map[string]bool)

		for dk, entryVal := range v.Entries {
			var kType string
			if v.KeyValues != nil {
				if actualKey, ok := v.KeyValues[dk]; ok && actualKey != nil {
					kType = valueOrThunkTypeNameWithDepth(actualKey, depth+1)
				}
			}
			if kType == "" {
				kType = dk.Type
			}
			keyTypesSet[kType] = true

			vType := valueOrThunkTypeNameWithDepth(entryVal, depth+1)
			valTypesSet[vType] = true
		}

		keyTypeStr := joinSortedTypeSet(keyTypesSet)
		valTypeStr := joinSortedTypeSet(valTypesSet)
		return fmt.Sprintf("{%s: %s}", keyTypeStr, valTypeStr)
	case *StructInstance:
		if len(v.TypeArgs) == 0 && v.Def != nil && len(v.Def.TypeParams) > 0 {
			inferStructInstanceTypeArgs(v)
		}
		if len(v.TypeArgs) > 0 {
			return fmt.Sprintf("%s[%s]", v.Def.Name, strings.Join(v.TypeArgs, ", "))
		}
		return v.Def.Name
	case *StructDefValue:
		if v.StructType != nil {
			if st, ok := v.StructType.(*types.StructType); ok {
				if initFn, ok := st.Methods["init"]; ok {
					return initFn.String()
				}
			}
		}
		if len(v.TypeArgs) > 0 {
			return fmt.Sprintf("() -> %s[%s]", v.Name, strings.Join(v.TypeArgs, ", "))
		}
		return "() -> " + v.Name

	case *Closure:
		if v.FuncType != nil {
			if t, ok := v.FuncType.(*types.FuncType); ok {
				return t.String()
			}
		}
		return "Function"
	case *BuiltinFunc:
		if v.FuncType != nil {
			if t, ok := v.FuncType.(*types.FuncType); ok {
				return t.String()
			}
		}
		return "Function"
	default:
		return "Unknown"
	}
}

func safeFieldTypeName(val Value) string {
	if evaluated, ok := PeekEvaluated(val); ok {
		return typeNameWithDepth(evaluated, 0)
	}
	if thunk, ok := val.(*Thunk); ok {
		if thunk.InferredType != nil {
			if t, ok := thunk.InferredType.(types.Type); ok && t != types.TypeUnknown {
				return t.String()
			}
		}
		if thunk.Expr != nil {
			if litVal, ok := evalLiteral(thunk.Expr); ok {
				return typeNameWithDepth(litVal, 0)
			}
			if tup, ok := thunk.Expr.(*ast.TupleLit); ok {
				if len(tup.Elements) == 0 {
					if tup.IsVariadic {
						return "(...)"
					}
					return "()"
				}
				hasSpread := false
				for _, el := range tup.Elements {
					if _, isSp := el.(*ast.SpreadExpr); isSp {
						hasSpread = true
						break
					}
				}
				if !hasSpread {
					parts := make([]string, len(tup.Elements))
					for i, el := range tup.Elements {
						if lv, ok := evalLiteral(el); ok {
							parts[i] = typeNameWithDepth(lv, 0)
						} else {
							parts[i] = "Unknown"
						}
					}
					if tup.IsVariadic {
						return "(" + strings.Join(parts, ", ") + ", ...)"
					}
					return "(" + strings.Join(parts, ", ") + ")"
				}
			}
		}
	}
	return ""
}

func matchTypeStringToField(declaredType types.Type, valTypeStr string, bindings map[string]string) {
	if declaredType == nil || valTypeStr == "" || valTypeStr == "Unknown" {
		return
	}
	switch dt := declaredType.(type) {
	case *types.TypeParamType:
		if _, ok := bindings[dt.Name]; !ok {
			bindings[dt.Name] = valTypeStr
		}
	case *types.TupleType:
		if strings.HasPrefix(valTypeStr, "(") && strings.HasSuffix(valTypeStr, ")") {
			inner := strings.TrimSpace(valTypeStr[1 : len(valTypeStr)-1])
			if inner != "" && inner != "..." {
				parts := splitTopLevelCommas(inner)
				if len(parts) > 0 && parts[len(parts)-1] == "..." {
					parts = parts[:len(parts)-1]
				}
				if dt.IsVariadic && len(dt.Elems) > 0 {
					varElem := dt.Elems[len(dt.Elems)-1]
					for _, p := range parts {
						matchTypeStringToField(varElem, p, bindings)
					}
				} else {
					for i := 0; i < len(dt.Elems) && i < len(parts); i++ {
						matchTypeStringToField(dt.Elems[i], parts[i], bindings)
					}
				}
			}
		}
	case *types.DictType:
		if strings.HasPrefix(valTypeStr, "{") && strings.HasSuffix(valTypeStr, "}") {
			inner := strings.TrimSpace(valTypeStr[1 : len(valTypeStr)-1])
			colonIdx := -1
			pDepth := 0
			bDepth := 0
			brDepth := 0
			for i, r := range inner {
				if r == '(' {
					pDepth++
				} else if r == ')' {
					pDepth--
				} else if r == '[' {
					bDepth++
				} else if r == ']' {
					bDepth--
				} else if r == '{' {
					brDepth++
				} else if r == '}' {
					brDepth--
				} else if r == ':' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
					colonIdx = i
					break
				}
			}
			if colonIdx != -1 {
				kStr := strings.TrimSpace(inner[:colonIdx])
				vStr := strings.TrimSpace(inner[colonIdx+1:])
				matchTypeStringToField(dt.Key, kStr, bindings)
				matchTypeStringToField(dt.Value, vStr, bindings)
			}
		}
	case *types.StructType:
		base, args, isGen := parseGenericTypeAnnot(valTypeStr)
		if isGen && dt.Name == base {
			for i := 0; i < len(dt.TypeArgs) && i < len(args); i++ {
				matchTypeStringToField(dt.TypeArgs[i], args[i], bindings)
			}
		}
	}
}

func inferStructInstanceTypeArgs(inst *StructInstance) {
	if inst.Def == nil || len(inst.Def.TypeParams) == 0 {
		return
	}
	if len(inst.TypeArgs) > 0 {
		return
	}
	bindings := make(map[string]string)
	if inst.Def.StructType != nil {
		if st, ok := inst.Def.StructType.(*types.StructType); ok {
			for fName, fType := range st.Fields {
				if fVal, ok := inst.Fields[fName]; ok {
					tName := safeFieldTypeName(fVal)
					if tName != "" && tName != "Unknown" {
						matchTypeStringToField(fType, tName, bindings)
					}
				}
			}
		}
	}
	res := make([]string, len(inst.Def.TypeParams))
	allFound := true
	for i, tp := range inst.Def.TypeParams {
		if b, ok := bindings[tp.Name]; ok {
			res[i] = b
		} else {
			allFound = false
			break
		}
	}
	if allFound {
		inst.TypeArgs = res
	}
}

func specializeGenericValue(val Value, typeArgs []string, env *Environment) (Value, error) {
	var inf *types.Inferencer
	if env != nil && env.Inferencer != nil {
		if i, ok := env.Inferencer.(*types.Inferencer); ok {
			inf = i
		}
	}

	parseArgType := func(s string) types.Type {
		if inf != nil {
			if t, err := inf.ParseTypeAnnot(s); err == nil {
				return t
			}
		}
		if t, err := types.ParseTypeAnnot(s); err == nil {
			return t
		}
		return types.TypeUnknown
	}

	switch v := val.(type) {
	case *StructDefValue:
		newDef := *v
		newDef.TypeArgs = append([]string(nil), typeArgs...)
		if v.StructType != nil {
			if st, ok := v.StructType.(*types.StructType); ok && len(st.TypeParams) > 0 {
				argTypes := make([]types.Type, len(typeArgs))
				bindings := make(map[string]types.Type)
				for i, argStr := range typeArgs {
					argT := parseArgType(argStr)
					argTypes[i] = argT
					if i < len(st.TypeParams) {
						bindings[st.TypeParams[i].Name] = argT
					}
				}
				if len(argTypes) == len(st.TypeParams) {
					newDef.StructType = types.InstantiateStruct(st, argTypes)
				} else {
					newDef.StructType = types.Substitute(st, bindings)
				}
			}
		}
		return &newDef, nil

	case *Closure:
		newClosure := *v
		newClosure.TypeArgs = append([]string(nil), typeArgs...)
		if v.FuncType != nil {
			if ft, ok := v.FuncType.(*types.FuncType); ok && len(ft.TypeParams) > 0 {
				bindings := make(map[string]types.Type)
				for i, argStr := range typeArgs {
					if i < len(ft.TypeParams) {
						bindings[ft.TypeParams[i].Name] = parseArgType(argStr)
					}
				}
				if len(bindings) > 0 {
					newClosure.FuncType = types.Substitute(ft, bindings)
				}
			}
		}
		return &newClosure, nil

	case *InterfaceDefValue:
		newIDef := *v
		newIDef.TypeArgs = append([]string(nil), typeArgs...)
		return &newIDef, nil

	default:
		return val, nil
	}
}

func parseGenericTypeAnnot(annot string) (string, []string, bool) {
	annot = strings.TrimSpace(annot)
	if !strings.HasSuffix(annot, "]") {
		return annot, nil, false
	}
	depth := 0
	bracketIdx := -1
	runes := []rune(annot)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == ']' {
			depth++
		} else if runes[i] == '[' {
			depth--
			if depth == 0 {
				bracketIdx = i
				break
			}
		}
	}
	if bracketIdx <= 0 {
		return annot, nil, false
	}
	base := strings.TrimSpace(string(runes[:bracketIdx]))
	if strings.Contains(base, "->") || strings.HasPrefix(base, "(") || strings.HasPrefix(base, "{") {
		return annot, nil, false
	}
	inner := strings.TrimSpace(string(runes[bracketIdx+1 : len(runes)-1]))
	if inner == "" {
		return annot, nil, false
	}
	args := splitTopLevelCommas(inner)
	return base, args, true
}

func substituteInterfaceDef(iface *InterfaceDefValue, typeArgs []string) *InterfaceDefValue {
	substMap := make(map[string]string)
	for i, tp := range iface.TypeParams {
		if i < len(typeArgs) {
			substMap[tp.Name] = typeArgs[i]
		}
	}
	var newEmbedded []string
	for _, emb := range iface.Embedded {
		newEmbedded = append(newEmbedded, substituteTypeString(emb, substMap))
	}
	res := &InterfaceDefValue{
		Name:       iface.Name,
		ModuleName: iface.ModuleName,
		TypeParams: iface.TypeParams,
		TypeArgs:   append([]string(nil), typeArgs...),
		Embedded:   newEmbedded,
		Methods:    make(map[string]ast.MethodSig),
	}
	for mName, sig := range iface.Methods {
		newSig := sig
		newSig.ReturnType = substituteTypeString(sig.ReturnType, substMap)
		newParams := make([]ast.FuncParam, len(sig.Params))
		for i, p := range sig.Params {
			newP := p
			newP.TypeAnnot = substituteTypeString(p.TypeAnnot, substMap)
			newParams[i] = newP
		}
		newSig.Params = newParams
		res.Methods[mName] = newSig
	}
	return res
}

func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func substituteTypeString(annot string, substMap map[string]string) string {
	if annot == "" || len(substMap) == 0 {
		return annot
	}
	var b strings.Builder
	runes := []rune(annot)
	for i := 0; i < len(runes); {
		if isIdentStart(runes[i]) {
			start := i
			for i < len(runes) && isIdentPart(runes[i]) {
				i++
			}
			word := string(runes[start:i])
			if repl, ok := substMap[word]; ok {
				b.WriteString(repl)
			} else {
				b.WriteString(word)
			}
		} else {
			b.WriteRune(runes[i])
			i++
		}
	}
	return b.String()
}

func (ev *Evaluator) EvalProgram(prog *ast.Program) (val Value, err error) {
	if prog != nil && prog.Filename != "" {
		if ev.globalEnv != nil {
			ev.globalEnv.Filename = prog.Filename
		}
		if ev.builtinsEnv != nil {
			ev.builtinsEnv.Filename = prog.Filename
		}
	}
	if ev.inferencer == nil {
		inf := types.NewInferencer()
		if ev.fileModuleLoader != nil {
			inf.SetModuleLoader(types.NewFileModuleLoader(ev.fileModuleLoader.baseDir).Load)
		}
		_ = inf.InferProgram(prog)
		ev.SetInferencer(inf)
	}
	defer func() {
		if r := recover(); r != nil {
			if exitErr, ok := r.(*ExitError); ok {
				// Normal termination via exit() or top-level return: flush all bufio writers
				flushErr := ev.globalEnv.FlushAllBufioWriters()
				if flushErr != nil {
					ev.globalEnv.ResetBufioWritersOnError()
					if exitErr.Code == 0 {
						err = flushErr
						return
					}
				}
				err = exitErr
				return
			}
			ev.globalEnv.ResetBufioWritersOnError()
			if retSig, ok := r.(*ReturnSignal); ok {
				err = FormatRuntimeError(retSig.Filename, retSig.Pos, fmt.Errorf("return outside of function"))
				return
			}
			if brkSig, ok := r.(*BreakSignal); ok {
				err = FormatRuntimeError(brkSig.Filename, brkSig.Pos, fmt.Errorf("break outside of closure"))
				return
			}
			// Runtime error (panic): A3: do NOT flush on runtime error
			rawMsg := fmt.Sprintf("%v", r)
			const thunkPrefix = "runtime error in thunk: "
			if strings.HasPrefix(rawMsg, thunkPrefix) {
				rawMsg = strings.TrimPrefix(rawMsg, thunkPrefix)
			}
			err = fmt.Errorf("%s", rawMsg)
			return
		}
		// Normal completion without panic/exit
		if err == nil {
			if flushErr := ev.globalEnv.FlushAllBufioWriters(); flushErr != nil {
				ev.globalEnv.ResetBufioWritersOnError()
				err = flushErr
			}
		} else {
			ev.globalEnv.ResetBufioWritersOnError()
		}
	}()

	var lastVal Value = &TupleValue{Elements: []Value{}}
	for _, stmt := range prog.Body {
		v, e := ev.globalEnv.evalStmt(stmt)
		if e != nil {
			return nil, FormatRuntimeError(ev.globalEnv.Filename, stmt.Pos(), e)
		}
		if v != nil {
			lastVal = v
		}
	}
	return lastVal, nil
}

func (env *Environment) isForceDemand(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.ForceExpr:
		return true
	}
	return false
}

func evalLiteral(expr ast.Expr) (Value, bool) {
	if expr == nil {
		return nil, false
	}
	switch lit := expr.(type) {
	case *ast.IntLit:
		if lit.BigValue != nil {
			return NewIntValueFromBig(lit.BigValue), true
		}
		return NewIntValue(lit.Value), true
	case *ast.FloatLit:
		return FloatValue(lit.Value), true
	case *ast.StringLit:
		return StringValue(lit.Value), true
	case *ast.BoolLit:
		return BoolValue(lit.Value), true
	case *ast.NilLit:
		return NilValue{}, true
	case *ast.UnaryExpr:
		if lit.Op == "-" {
			if intLit, ok := lit.Expr.(*ast.IntLit); ok {
				if intLit.BigValue != nil {
					neg := new(big.Int).Neg(intLit.BigValue)
					return NewIntValueFromBig(neg), true
				}
				if intLit.Value == math.MinInt64 {
					neg := new(big.Int).Neg(big.NewInt(math.MinInt64))
					return NewIntValueFromBig(neg), true
				}
				return NewIntValue(-intLit.Value), true
			}
			if floatLit, ok := lit.Expr.(*ast.FloatLit); ok {
				return FloatValue(-floatLit.Value), true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func (env *Environment) newThunk(expr ast.Expr) *Thunk {
	thunk := NewThunk(expr, env)
	if expr != nil && env != nil && env.Inferencer != nil {
		if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
			if t, ok := inf.ExprTypes[expr]; ok {
				thunk.InferredType = t
			}
		}
	}
	return thunk
}

func (env *Environment) evalStmt(stmt ast.Stmt) (Value, error) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		if _, ok := env.store[s.Name]; ok {
			// Re-binding existing variable in current environment:
			// Force RHS immediately to avoid circular reference in lazy thunk
			newVal := Force(env.newThunk(s.Value))
			env.store[s.Name] = newVal
			return nil, nil
		}
		// If RHS is a constant literal, evaluate immediately without lazy thunk!
		if val, ok := evalLiteral(s.Value); ok {
			env.Set(s.Name, val)
			return nil, nil
		}
		// If RHS is a force demand point, force immediately instead of creating lazy thunk!
		if env.isForceDemand(s.Value) {
			val := Force(env.newThunk(s.Value))
			env.Set(s.Name, val)
			return nil, nil
		}
		// Wrap RHS in lazy Thunk!
		thunk := env.newThunk(s.Value)
		env.Set(s.Name, thunk)
		return nil, nil

	case *ast.UnpackDecl:
		val := Force(NewThunk(s.Value, env))
		tVal, ok := val.(*TupleValue)
		if !ok {
			return nil, fmt.Errorf("cannot unpack non-tuple value: %v", val)
		}
		reqCount := len(s.Pattern.Elements)
		if s.Pattern.RestName != "" {
			if !tVal.IsVariadic && len(tVal.Elements) < reqCount {
				return nil, fmt.Errorf("not enough elements to unpack into tuple pattern: got %d, need at least %d", len(tVal.Elements), reqCount)
			}
		} else {
			if tVal.IsVariadic {
				return nil, fmt.Errorf("cannot unpack infinite tuple into fixed-length pattern")
			}
			if len(tVal.Elements) != reqCount {
				return nil, fmt.Errorf("element count mismatch in unpack: expected %d, got %d", reqCount, len(tVal.Elements))
			}
		}
		for i, name := range s.Pattern.Elements {
			if name != "_" {
				if i < len(tVal.Elements) {
					env.Set(name, tVal.Elements[i])
				} else if tVal.IsVariadic && len(tVal.Elements) > 0 {
					env.Set(name, tVal.Elements[len(tVal.Elements)-1])
				} else {
					env.Set(name, &TupleValue{Elements: []Value{}})
				}
			}
		}
		if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
			var restElements []Value
			if reqCount < len(tVal.Elements) {
				restElements = tVal.Elements[reqCount:]
			}
			env.Set(s.Pattern.RestName, &TupleValue{Elements: restElements, IsVariadic: tVal.IsVariadic})
		}
		return nil, nil

	case *ast.FuncDef:
		closure := &Closure{
			TypeParams: s.TypeParams,
			Params:     s.Params,
			BlockParam: s.BlockParam,
			Body:       s.Body,
			Env:        env,
			IsFuncDef:  true,
		}
		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok {
				if inf.StmtTypes != nil {
					if fnType, ok := inf.StmtTypes[s]; ok {
						if ft, isFT := fnType.(*types.FuncType); isFT {
							closure.FuncType = ft
						}
					}
				}
				if closure.FuncType == nil {
					if fnType, ok := inf.Scope().Lookup(s.Name); ok {
						if ft, isFT := fnType.(*types.FuncType); isFT {
							closure.FuncType = ft
						}
					}
				}
			}
		}
		env.Set(s.Name, closure)
		return nil, nil

	case *ast.StructDef:
		sDef := &StructDefValue{
			Name:          s.Name,
			TypeParams:    s.TypeParams,
			FieldOrder:    make([]string, 0),
			FieldDefaults: make(map[string]ast.Expr),
			EmbeddedNames: make([]string, 0),
			Methods:       make(map[string]*ast.FuncDef),
			AllMethods:    make(map[string][]*ast.FuncDef),
			Env:           env,
		}

		for _, f := range s.Fields {
			if f.IsEmbedded {
				embName := f.TypeAnnot
				if embName == "" {
					embName = f.Name
				}
				sDef.EmbeddedNames = append(sDef.EmbeddedNames, embName)
				sDef.FieldOrder = append(sDef.FieldOrder, f.Name)
			} else {
				sDef.FieldOrder = append(sDef.FieldOrder, f.Name)
				if f.DefaultValue != nil {
					sDef.FieldDefaults[f.Name] = f.DefaultValue
				}
			}
		}

		for _, m := range s.Methods {
			sDef.Methods[m.Name] = m
			sDef.AllMethods[m.Name] = append(sDef.AllMethods[m.Name], m)
		}

		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok {
				if inf.StmtTypes != nil {
					if st, ok := inf.StmtTypes[s]; ok {
						if structT, isST := st.(*types.StructType); isST {
							sDef.StructType = structT
						}
					}
				}
				if sDef.StructType == nil {
					if st, ok := inf.Scope().Lookup(s.Name); ok {
						if structT, isST := st.(*types.StructType); isST {
							sDef.StructType = structT
						}
					}
				}
			}
		}

		env.Set(s.Name, sDef)
		return nil, nil

	case *ast.InterfaceDef:
		iDef := &InterfaceDefValue{
			Name:       s.Name,
			TypeParams: s.TypeParams,
			Embedded:   s.Embedded,
			Methods:    make(map[string]ast.MethodSig),
		}
		for _, m := range s.Methods {
			iDef.Methods[m.Name] = m
		}
		env.Set(s.Name, iDef)
		return nil, nil

	case *ast.MemberAssignStmt:
		targetVal := Force(NewThunk(s.Target, env))
		si, ok := targetVal.(*StructInstance)
		if !ok {
			return nil, fmt.Errorf("cannot assign member '%s' on non-struct %s", s.Member, targetVal.String())
		}
		if !si.IsInitializing {
			return nil, fmt.Errorf("cannot mutate field '%s' of immutable struct '%s'", s.Member, si.Def.Name)
		}
		found := false
		for _, fName := range si.Def.FieldOrder {
			if fName == s.Member {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("struct '%s' has no field '%s'", si.Def.Name, s.Member)
		}
		val, err := env.evalExpr(s.Value)
		if err != nil {
			return nil, err
		}
		si.Fields[s.Member] = val
		return nil, nil

	case *ast.ReturnStmt:
		var retVal Value = &TupleValue{Elements: []Value{}}
		if s.Value != nil {
			v, err := env.evalExpr(s.Value)
			if err != nil {
				return nil, err
			}
			retVal = v
		}

		if env.EnclosingFuncID == 0 {
			// Top-level return behaves like exit()
			code := 0
			forced := Force(retVal)
			switch v := forced.(type) {
			case *TupleValue:
				if len(v.Elements) == 0 && !v.IsVariadic {
					code = 0
				} else {
					return nil, fmt.Errorf("return at top level expects Int or Bool, got %v", forced)
				}
			case IntValue:
				if c, ok := v.ToInt(); ok {
					code = c
				} else {
					low := int(v.ToBig().Int64())
					if low != 0 {
						code = low
					} else {
						code = 1
					}
				}
			case BoolValue:
				if bool(v) {
					code = 0
				} else {
					code = 1
				}
			default:
				return nil, fmt.Errorf("return at top level expects Int or Bool, got %v", forced)
			}
			panic(&ExitError{Code: code})
		}

		panic(&ReturnSignal{
			FuncID:   env.EnclosingFuncID,
			Value:    retVal,
			Pos:      s.Pos(),
			Filename: env.Filename,
		})

	case *ast.BreakStmt:
		var brkVal Value = &TupleValue{Elements: []Value{}}
		if s.Value != nil {
			v, err := env.evalExpr(s.Value)
			if err != nil {
				return nil, err
			}
			brkVal = v
		}

		if env.ActiveLoopID != 0 {
			panic(&BreakSignal{
				LoopID:   env.ActiveLoopID,
				Value:    brkVal,
				Pos:      s.Pos(),
				Filename: env.Filename,
			})
		}
		if env.ActiveClosureID != 0 {
			panic(&BreakSignal{
				ClosureID: env.ActiveClosureID,
				Value:     brkVal,
				Pos:       s.Pos(),
				Filename:  env.Filename,
			})
		}
		return nil, FormatRuntimeError(env.Filename, s.Pos(), fmt.Errorf("break outside of closure or loop"))

	case *ast.ContinueStmt:
		if env.ActiveLoopID != 0 {
			panic(&ContinueSignal{
				LoopID:   env.ActiveLoopID,
				Pos:      s.Pos(),
				Filename: env.Filename,
			})
		}
		if env.ActiveClosureID != 0 {
			panic(&ContinueSignal{
				ClosureID: env.ActiveClosureID,
				Pos:       s.Pos(),
				Filename:  env.Filename,
			})
		}
		return nil, FormatRuntimeError(env.Filename, s.Pos(), fmt.Errorf("continue outside of loop"))

	case *ast.ForStmt:
		return env.evalForStmt(s)

	case *ast.IfStmt:
		condVal := Force(NewThunk(s.Cond, env))
		bVal, ok := condVal.(BoolValue)
		if !ok {
			return nil, fmt.Errorf("condition must be Bool, got %v", condVal)
		}
		if bool(bVal) {
			return env.evalBlock(s.ThenBody)
		} else if len(s.ElseBody) > 0 {
			return env.evalBlock(s.ElseBody)
		}
		return nil, nil

	case *ast.ExprStmt:
		val, err := env.evalExpr(s.Value)
		if err != nil {
			return nil, err
		}
		// If the statement is a bare identifier/member resolving to a function,
		// auto-invoke it with 0 arguments (e.g. bare 'puts' for newline).
		isCallableTarget := false
		switch s.Value.(type) {
		case *ast.Identifier, *ast.ModuleAccessExpr:
			isCallableTarget = true
		}
		if isCallableTarget {
			forced := Force(val)
			switch fn := forced.(type) {
			case *BuiltinFunc:
				return fn.Fn([]Value{})
			case *Closure:
				return env.invokeClosure(fn, []Value{})
			}
		}
		return val, nil

	case *ast.ModuleDecl:
		return nil, nil

	case *ast.ImportStmt:
		alias := s.Alias
		if existingMod, ok := env.GetModule(s.Path); ok {
			if alias != "" && alias != existingMod.Name {
				aliasedMod := &ModuleValue{
					Name:    alias,
					Exports: existingMod.Exports,
				}
				env.RegisterModule(aliasedMod)
			}
			return nil, nil
		}
		if env.Loader == nil {
			return nil, fmt.Errorf("module loader not configured")
		}
		modVal, err := env.Loader(s.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to import module '%s': %w", s.Path, err)
		}
		canonicalName := modVal.Name
		if alias == "" {
			alias = modVal.Name
		}
		// Register module with alias name
		modVal.Name = alias
		env.RegisterModule(modVal)
		if s.Path != alias {
			env.RegisterModule(&ModuleValue{Name: s.Path, Exports: modVal.Exports})
		}
		if canonicalName != "" && canonicalName != alias && canonicalName != s.Path {
			env.RegisterModule(&ModuleValue{Name: canonicalName, Exports: modVal.Exports})
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown statement: %T", stmt)
	}
}

func (env *Environment) evalBlock(body []ast.Stmt) (Value, error) {
	var lastVal Value = &TupleValue{Elements: []Value{}}
	for _, stmt := range body {
		v, err := env.evalStmt(stmt)
		if err != nil {
			return nil, FormatRuntimeError(env.Filename, stmt.Pos(), err)
		}
		if v != nil {
			lastVal = v
		}
	}
	return lastVal, nil
}

func (env *Environment) invokeClosure(fn *Closure, argValues []Value, optBlock ...Value) (val Value, err error) {
	callEnv := NewEnvironment(fn.Env)
	var myCallID uint64

	if fn.IsFuncDef {
		// Regular function call (def)
		myCallID = nextCallID()
		callEnv.EnclosingFuncID = myCallID
		callEnv.ActiveClosureID = 0
	} else {
		// Closure / lambda / block call
		myCallID = nextCallID()
		callEnv.EnclosingFuncID = fn.EnclosingFuncID
		callEnv.ActiveClosureID = myCallID
	}

	for i, p := range fn.Params {
		if p.IsVariadic {
			var rest []Value
			if i < len(argValues) {
				rest = argValues[i:]
			}
			callEnv.Set(p.Name, &TupleValue{Elements: rest})
			break
		}
		if i < len(argValues) {
			callEnv.Set(p.Name, argValues[i])
		} else if p.DefaultValue != nil {
			callEnv.Set(p.Name, NewThunk(p.DefaultValue, callEnv))
		} else {
			callEnv.Set(p.Name, &TupleValue{Elements: []Value{}})
		}
	}

	if fn.BlockParam != nil {
		var blockVal Value
		if len(optBlock) > 0 && optBlock[0] != nil {
			blockVal = optBlock[0]
		}
		if blockVal != nil {
			callEnv.Set(fn.BlockParam.Name, blockVal)
		} else if fn.BlockParam.DefaultValue != nil {
			callEnv.Set(fn.BlockParam.Name, NewThunk(fn.BlockParam.DefaultValue, callEnv))
		} else {
			return nil, fmt.Errorf("missing required block argument &%s", fn.BlockParam.Name)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if fn.IsFuncDef {
				if ret, ok := r.(*ReturnSignal); ok {
					if ret.FuncID == myCallID {
						val = ret.Value
						err = nil
						return
					}
					panic(ret)
				}
			} else {
				if brk, ok := r.(*BreakSignal); ok {
					if brk.ClosureID == myCallID {
						val = brk.Value
						err = nil
						return
					}
					panic(brk)
				}
				if cont, ok := r.(*ContinueSignal); ok {
					if cont.ClosureID == myCallID {
						val = &TupleValue{Elements: []Value{}}
						err = nil
						return
					}
					panic(cont)
				}
			}
			panic(r)
		}
	}()

	return callEnv.evalBlock(fn.Body)
}

func (env *Environment) evalForStmt(s *ast.ForStmt) (Value, error) {
	loopID := nextCallID()
	prevLoopID := env.ActiveLoopID
	env.ActiveLoopID = loopID
	defer func() {
		env.ActiveLoopID = prevLoopID
	}()

	var breakVal Value = &TupleValue{Elements: []Value{}}

	for {
		if s.Cond != nil {
			condVal := Force(NewThunk(s.Cond, env))
			bVal, ok := condVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("for condition must be Bool, got %v", condVal)
			}
			if !bool(bVal) {
				break
			}
		}

		var broke bool
		var loopErr error

		func() {
			defer func() {
				if r := recover(); r != nil {
					if brk, ok := r.(*BreakSignal); ok {
						if brk.LoopID == loopID || brk.ClosureID == loopID {
							broke = true
							if brk.Value != nil {
								breakVal = brk.Value
							}
							return
						}
					}
					if cont, ok := r.(*ContinueSignal); ok {
						if cont.LoopID == loopID || cont.ClosureID == loopID {
							return // continue to next iteration
						}
					}
					// propagate return / outer break / panic
					panic(r)
				}
			}()

			for _, stmt := range s.Body {
				if _, err := env.evalStmt(stmt); err != nil {
					loopErr = err
					return
				}
			}
		}()

		if loopErr != nil {
			return nil, loopErr
		}
		if broke {
			break
		}
	}

	return breakVal, nil
}

func (env *Environment) evalExpr(expr ast.Expr) (Value, error) {
	if expr == nil {
		return &TupleValue{Elements: []Value{}}, nil
	}

	switch e := expr.(type) {
	case *ast.IntLit:
		if e.BigValue != nil {
			return NewIntValueFromBig(e.BigValue), nil
		}
		return NewIntValue(e.Value), nil
	case *ast.FloatLit:
		return FloatValue(e.Value), nil
	case *ast.StringLit:
		return StringValue(e.Value), nil
	case *ast.BoolLit:
		return BoolValue(e.Value), nil
	case *ast.NilLit:
		return NilValue{}, nil
	case *ast.ForceExpr:
		return Force(NewThunk(e.Expr, env)), nil

	case *ast.Identifier:
		v, ok := env.Get(e.Name)
		if !ok {
			return nil, fmt.Errorf("undefined identifier: %s", e.Name)
		}
		return v, nil

	case *ast.TupleLit:
		// Lazy tuple elements: wrapped in Thunks unless force-demanded or constant literal
		elems := make([]Value, 0, len(e.Elements))
		isVariadic := e.IsVariadic
		for _, el := range e.Elements {
			if sp, ok := el.(*ast.SpreadExpr); ok {
				spVal := Force(env.newThunk(sp.Expr))
				spTuple, ok := spVal.(*TupleValue)
				if !ok {
					return nil, fmt.Errorf("cannot spread non-tuple value of type %s", typeNameOf(spVal))
				}
				elems = append(elems, spTuple.Elements...)
				if spTuple.IsVariadic {
					isVariadic = true
				}
			} else {
				if env.isForceDemand(el) {
					elems = append(elems, Force(env.newThunk(el)))
				} else if val, ok := evalLiteral(el); ok {
					elems = append(elems, val)
				} else {
					elems = append(elems, env.newThunk(el))
				}
			}
		}
		return &TupleValue{Elements: elems, IsVariadic: isVariadic}, nil

	case *ast.DictLit:
		entries := make(map[DictKey]Value)
		keyValues := make(map[DictKey]Value)
		for _, entry := range e.Entries {
			kVal := Force(env.newThunk(entry.Key))
			dk := MakeDictKey(kVal)
			keyValues[dk] = kVal
			if env.isForceDemand(entry.Value) {
				entries[dk] = Force(env.newThunk(entry.Value))
			} else if val, ok := evalLiteral(entry.Value); ok {
				entries[dk] = val
			} else {
				entries[dk] = env.newThunk(entry.Value)
			}
		}
		return NewDictValue(entries, keyValues), nil

	case *ast.TypeInstExpr:
		targetVal := Force(NewThunk(e.Target, env))
		return specializeGenericValue(targetVal, e.TypeArgs, env)

	case *ast.IndexExpr:
		if e.TypeArg != "" {
			targetVal := Force(NewThunk(e.Target, env))
			switch targetVal.(type) {
			case *StructDefValue, *Closure, *InterfaceDefValue:
				return specializeGenericValue(targetVal, []string{e.TypeArg}, env)
			}
		}
		targetVal := Force(NewThunk(e.Target, env))
		idxVal := Force(NewThunk(e.Index, env))

		if si, ok := targetVal.(*StructInstance); ok {
			if fn, err := si.LookupMethod("[]", 1); err == nil {
				return fn.Env.invokeClosure(fn, []Value{idxVal})
			}
			if m, err := si.LookupMember("[]"); err == nil {
				forced := Force(m)
				if fn, ok := forced.(*Closure); ok {
					return fn.Env.invokeClosure(fn, []Value{idxVal})
				}
				if bf, ok := forced.(*BuiltinFunc); ok {
					return bf.Fn([]Value{idxVal})
				}
			}
		}

		switch t := targetVal.(type) {
		case *TupleValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("tuple index must be Int, got %v", idxVal)
			}
			idx, ok := i.ToInt()
			if !ok {
				return nil, fmt.Errorf("tuple index out of bounds: %s (len %d)", i.String(), len(t.Elements))
			}
			if idx < 0 {
				if t.IsVariadic {
					return nil, fmt.Errorf("cannot use negative index on infinite tuple: %s", i.String())
				}
				idx = len(t.Elements) + idx
			}
			if idx < 0 || idx >= len(t.Elements) {
				if t.IsVariadic && len(t.Elements) > 0 && idx >= len(t.Elements) {
					return t.Elements[len(t.Elements)-1], nil
				}
				return nil, fmt.Errorf("tuple index out of bounds: %s (len %d)", i.String(), len(t.Elements))
			}
			return t.Elements[idx], nil
		case StringValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("string index must be Int, got %v", idxVal)
			}
			runes := []rune(string(t))
			idx, ok := i.ToInt()
			if !ok {
				return nil, fmt.Errorf("string index out of bounds: %s (len %d)", i.String(), len(runes))
			}
			if idx < 0 {
				idx = len(runes) + idx
			}
			if idx < 0 || idx >= len(runes) {
				return nil, fmt.Errorf("string index out of bounds: %s (len %d)", i.String(), len(runes))
			}
			return StringValue(string(runes[idx])), nil
		case BytesValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("bytes index must be Int, got %v", idxVal)
			}
			data := []byte(t)
			idx, ok := i.ToInt()
			if !ok {
				return nil, fmt.Errorf("bytes index out of bounds: %s (len %d)", i.String(), len(data))
			}
			if idx < 0 {
				idx = len(data) + idx
			}
			if idx < 0 || idx >= len(data) {
				return nil, fmt.Errorf("bytes index out of bounds: %s (len %d)", i.String(), len(data))
			}
			return NewIntValue(int64(data[idx])), nil
		case *DictValue:
			dk := MakeDictKey(idxVal)
			if v, ok := t.Entries[dk]; ok {
				return v, nil
			}
			return nil, fmt.Errorf("key not found in dict: %s", idxVal.String())
		default:
			return nil, fmt.Errorf("cannot index non-tuple/non-string/non-bytes/non-dict value: %v", targetVal)
		}

	case *ast.BinaryExpr:
		if e.Op == "&&" {
			leftVal := Force(NewThunk(e.Left, env))
			lb, ok := leftVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator && expects Bool, got %v", leftVal)
			}
			if !bool(lb) {
				return BoolValue(false), nil
			}
			rightVal := Force(NewThunk(e.Right, env))
			rb, ok := rightVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator && expects Bool, got %v", rightVal)
			}
			return BoolValue(bool(rb)), nil
		}
		if e.Op == "||" {
			leftVal := Force(NewThunk(e.Left, env))
			lb, ok := leftVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator || expects Bool, got %v", leftVal)
			}
			if bool(lb) {
				return BoolValue(true), nil
			}
			rightVal := Force(NewThunk(e.Right, env))
			rb, ok := rightVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator || expects Bool, got %v", rightVal)
			}
			return BoolValue(bool(rb)), nil
		}
		if e.Op == "<<" {
			leftVal := Force(NewThunk(e.Left, env))
			if lt, ok := leftVal.(*TupleValue); ok {
				var rightVal Value
				if env.isForceDemand(e.Right) {
					rightVal = Force(env.newThunk(e.Right))
				} else if val, ok := evalLiteral(e.Right); ok {
					rightVal = val
				} else {
					rightVal = env.newThunk(e.Right)
				}
				newElems := append(append([]Value(nil), lt.Elements...), rightVal)
				return &TupleValue{Elements: newElems, IsVariadic: lt.IsVariadic}, nil
			}
			rightVal := Force(NewThunk(e.Right, env))
			return evalBinary(e.Op, leftVal, rightVal)
		}
		leftVal := Force(NewThunk(e.Left, env))
		rightVal := Force(NewThunk(e.Right, env))
		return evalBinary(e.Op, leftVal, rightVal)

	case *ast.UnaryExpr:
		val := Force(NewThunk(e.Expr, env))
		if si, ok := val.(*StructInstance); ok {
			if fn, err := si.LookupMethod(e.Op, 0); err == nil {
				return fn.Env.invokeClosure(fn, []Value{})
			}
			if m, err := si.LookupMember(e.Op); err == nil {
				forced := Force(m)
				if fn, ok := forced.(*Closure); ok {
					return fn.Env.invokeClosure(fn, []Value{})
				}
				if bf, ok := forced.(*BuiltinFunc); ok {
					return bf.Fn([]Value{})
				}
			}
		}
		switch e.Op {
		case "+":
			if iv, ok := val.(IntValue); ok {
				return iv, nil
			}
			if fv, ok := val.(FloatValue); ok {
				return fv, nil
			}
			return nil, fmt.Errorf("unary + expects number, got %v", val)
		case "-":
			if iv, ok := val.(IntValue); ok {
				return iv.Neg(), nil
			}
			if fv, ok := val.(FloatValue); ok {
				return -fv, nil
			}
			return nil, fmt.Errorf("unary - expects number, got %v", val)
		case "!":
			if bv, ok := val.(BoolValue); ok {
				return !bv, nil
			}
			return nil, fmt.Errorf("unary ! expects Bool, got %v", val)
		case "~":
			if iv, ok := val.(IntValue); ok {
				return iv.Not(), nil
			}
			return nil, fmt.Errorf("unary ~ expects Int, got %v", val)
		default:
			return nil, fmt.Errorf("unknown unary operator %s", e.Op)
		}

	case *ast.TernaryExpr:
		condVal := Force(NewThunk(e.Cond, env))
		bVal, ok := condVal.(BoolValue)
		if !ok {
			return nil, fmt.Errorf("ternary condition must be Bool, got %v", condVal)
		}
		if bool(bVal) {
			return env.evalExpr(e.Then)
		}
		return env.evalExpr(e.Else)

	case *ast.CallExpr:
		calleeVal := Force(NewThunk(e.Callee, env))

		// Prepare argument thunks (pass-by-need) unless force-demanded or constant literal
		argValues := make([]Value, len(e.Args))
		for i, a := range e.Args {
			if env.isForceDemand(a) {
				argValues[i] = Force(env.newThunk(a))
			} else if val, ok := evalLiteral(a); ok {
				argValues[i] = val
			} else {
				argValues[i] = env.newThunk(a)
			}
		}

		// Trailing block closure
		var blockClosure *Closure
		if e.Block != nil {
			blockClosure = &Closure{
				Params:          e.Block.Params,
				Body:            e.Block.Body,
				Env:             env,
				IsFuncDef:       false,
				EnclosingFuncID: env.EnclosingFuncID,
			}
			if env != nil && env.Inferencer != nil {
				if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
					if fnType, ok := inf.ExprTypes[e.Block]; ok {
						blockClosure.FuncType = fnType
					}
				}
			}
		}

		switch fn := calleeVal.(type) {
		case *BuiltinFunc:
			if blockClosure != nil {
				argValues = append(argValues, blockClosure)
			}
			return fn.Fn(argValues)

		case *Closure:
			if fn.BlockParam != nil {
				if blockClosure != nil {
					return env.invokeClosure(fn, argValues, blockClosure)
				}
				return env.invokeClosure(fn, argValues)
			}
			if blockClosure != nil {
				argValues = append(argValues, blockClosure)
			}
			return env.invokeClosure(fn, argValues)

		case *StructDefValue:
			targetDef := fn
			if len(targetDef.TypeArgs) == 0 && len(targetDef.TypeParams) > 0 && env != nil && env.Inferencer != nil {
				if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
					if callT, ok := inf.ExprTypes[e]; ok {
						if st, ok := callT.(*types.StructType); ok && len(st.TypeArgs) == len(targetDef.TypeParams) {
							typeArgs := make([]string, len(st.TypeArgs))
							allKnown := true
							for i, a := range st.TypeArgs {
								if a == types.TypeUnknown {
									allKnown = false
									break
								}
								typeArgs[i] = a.String()
							}
							if allKnown {
								if sp, err := specializeGenericValue(targetDef, typeArgs, env); err == nil {
									if spDef, ok := sp.(*StructDefValue); ok {
										targetDef = spDef
									}
								}
							}
						}
					}
				}
			}
			return env.instantiateStruct(targetDef, e.Args)

		default:
			return nil, fmt.Errorf("cannot call non-function value: %v", calleeVal)
		}

	case *ast.MemberAccessExpr:
		targetVal := Force(NewThunk(e.Target, env))
		if si, ok := targetVal.(*StructInstance); ok {
			memVal, err := si.LookupMember(e.Member)
			if err == nil {
				if closure, ok := memVal.(*Closure); ok && closure.FuncType == nil {
					if si.Def != nil && si.Def.StructType != nil {
						if st, ok := si.Def.StructType.(*types.StructType); ok {
							if mType, err := st.LookupMember(e.Member); err == nil {
								if ft, isFT := mType.(*types.FuncType); isFT {
									closure.FuncType = ft
								}
							}
						}
					}
				}
			}
			return memVal, err
		}
		return lookupBuiltinMember(targetVal, e.Member)

	case *ast.NamedArgExpr:
		return env.evalExpr(e.Value)

	case *ast.ModuleAccessExpr:
		mod, ok := env.GetModule(e.Module)
		if !ok {
			return nil, fmt.Errorf("module '@%s' not found or not imported", e.Module)
		}
		// Enforce Go-style visibility: member must start with Uppercase letter to be public
		firstChar := e.Member[0]
		if firstChar < 'A' || firstChar > 'Z' {
			return nil, fmt.Errorf("cannot access private member '%s' from module '@%s'", e.Member, e.Module)
		}
		val, ok := mod.Exports[e.Member]
		if !ok {
			return nil, fmt.Errorf("member '%s' not exported by module '@%s'", e.Member, e.Module)
		}
		return val, nil

	case *ast.LambdaExpr:
		closure := &Closure{
			Params:          e.Params,
			Body:            e.Body,
			Env:             env,
			IsFuncDef:       false,
			EnclosingFuncID: env.EnclosingFuncID,
		}
		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
				if fnType, ok := inf.ExprTypes[e]; ok {
					closure.FuncType = fnType
				}
			}
		}
		return closure, nil

	case *ast.IsExpr:
		val := Force(NewThunk(e.Expr, env))
		matches := checkValueMatchesType(val, strings.TrimSpace(e.TypeAnnot), env)
		return BoolValue(matches), nil

	case *ast.MatchExpr:
		val := Force(NewThunk(e.Target, env))
		for _, c := range e.Cases {
			if c.TuplePattern != nil {
				tVal, ok := val.(*TupleValue)
				if !ok {
					continue
				}
				reqCount := len(c.TuplePattern.Elements)
				if c.TuplePattern.RestName != "" {
					if !tVal.IsVariadic && len(tVal.Elements) < reqCount {
						continue
					}
				} else {
					if tVal.IsVariadic {
						continue
					}
					if len(tVal.Elements) != reqCount {
						continue
					}
				}

				// Matched! Bind unpacked elements in a new scope
				caseEnv := NewEnvironment(env)
				for i, name := range c.TuplePattern.Elements {
					if name != "_" {
						if i < len(tVal.Elements) {
							caseEnv.Set(name, tVal.Elements[i])
						} else if tVal.IsVariadic && len(tVal.Elements) > 0 {
							caseEnv.Set(name, tVal.Elements[len(tVal.Elements)-1])
						}
					}
				}
				if c.TuplePattern.RestName != "" && c.TuplePattern.RestName != "_" {
					var restElements []Value
					if reqCount < len(tVal.Elements) {
						restElements = tVal.Elements[reqCount:]
					}
					caseEnv.Set(c.TuplePattern.RestName, &TupleValue{Elements: restElements, IsVariadic: tVal.IsVariadic})
				}
				return caseEnv.evalBlock(c.Body)
			}

			pattern := strings.TrimSpace(c.Pattern)
			if pattern == "_" || checkValueMatchesType(val, pattern, env) {
				return env.evalBlock(c.Body)
			}
		}
		return &TupleValue{Elements: []Value{}}, nil

	default:
		return nil, fmt.Errorf("unknown expression: %T", expr)
	}
}

func (env *Environment) instantiateStruct(sDef *StructDefValue, args []ast.Expr) (*StructInstance, error) {
	fields := make(map[string]Value)
	embedded := make(map[string]*StructInstance)

	var baseInstance *StructInstance
	argStartIdx := 0

	// Check if 1st argument is a copy-update base instance (e.g. Person(p, x: 30))
	if len(args) > 0 {
		if _, isNamed := args[0].(*ast.NamedArgExpr); !isNamed {
			th0 := env.newThunk(args[0])
			isPotentialBase := false
			if th0.InferredType != nil {
				if st, ok := th0.InferredType.(*types.StructType); ok && st.Name == sDef.Name {
					isPotentialBase = true
				}
			} else if ident, isIdent := args[0].(*ast.Identifier); isIdent {
				if v, ok := env.Get(ident.Name); ok {
					if evaluated, isEval := PeekEvaluated(v); isEval {
						if si, ok := evaluated.(*StructInstance); ok && si.Def.Name == sDef.Name {
							isPotentialBase = true
						}
					}
				}
			}
			if isPotentialBase {
				val := Force(th0)
				if si, ok := val.(*StructInstance); ok && si.Def.Name == sDef.Name {
					baseInstance = si
					argStartIdx = 1
				}
			}
		}
	}

	if baseInstance != nil {
		for k, v := range baseInstance.Fields {
			fields[k] = v
		}
		for k, v := range baseInstance.Embedded {
			embedded[k] = v
		}
		posIdx := 0
		for i := argStartIdx; i < len(args); i++ {
			arg := args[i]
			if named, ok := arg.(*ast.NamedArgExpr); ok {
				fields[named.Name] = env.newThunk(named.Value)
			} else {
				if posIdx < len(sDef.FieldOrder) {
					fName := sDef.FieldOrder[posIdx]
					fields[fName] = env.newThunk(arg)
					posIdx++
				}
			}
		}
		for _, embName := range sDef.EmbeddedNames {
			if fVal, ok := fields[embName]; ok {
				forced := Force(fVal)
				if si, ok := forced.(*StructInstance); ok {
					embedded[embName] = si
				}
			}
		}
		inst := &StructInstance{
			Def:      sDef,
			TypeArgs: append([]string(nil), sDef.TypeArgs...),
			Fields:   fields,
			Embedded: embedded,
		}
		if len(inst.TypeArgs) == 0 && len(sDef.TypeParams) > 0 {
			inferStructInstanceTypeArgs(inst)
		}
		return inst, nil
	}

	if initDef, hasInit := sDef.Methods["init"]; hasInit {
		return env.instantiateStructWithInit(sDef, initDef, args)
	}

	// Positional index for non-named arguments
	posIdx := 0

	for i := argStartIdx; i < len(args); i++ {
		arg := args[i]
		if named, ok := arg.(*ast.NamedArgExpr); ok {
			fields[named.Name] = env.newThunk(named.Value)
		} else {
			// Positional argument
			if posIdx < len(sDef.FieldOrder) {
				fName := sDef.FieldOrder[posIdx]
				fields[fName] = env.newThunk(arg)
				posIdx++
			}
		}
	}

	isEmbeddedField := make(map[string]bool)
	for _, embName := range sDef.EmbeddedNames {
		isEmbeddedField[embName] = true
		if base, _, isGen := parseGenericTypeAnnot(embName); isGen {
			isEmbeddedField[base] = true
		}
	}

	// For any fields not yet populated:
	for _, fName := range sDef.FieldOrder {
		if isEmbeddedField[fName] {
			continue
		}
		if _, ok := fields[fName]; !ok {
			if defExpr, hasDef := sDef.FieldDefaults[fName]; hasDef {
				fields[fName] = sDef.Env.newThunk(defExpr)
			} else {
				fields[fName] = &TupleValue{Elements: []Value{}}
			}
		}
	}

	// Populate embedded struct instances
	for _, embName := range sDef.EmbeddedNames {
		lookupName := embName
		if base, _, isGen := parseGenericTypeAnnot(embName); isGen {
			lookupName = base
		}
		if fVal, ok := fields[lookupName]; ok {
			forced := Force(fVal)
			if si, ok := forced.(*StructInstance); ok {
				embedded[lookupName] = si
			}
		} else if fVal, ok := fields[embName]; ok {
			forced := Force(fVal)
			if si, ok := forced.(*StructInstance); ok {
				embedded[lookupName] = si
			}
		} else {
			var embTypeArgs []string
			if base, args, isGen := parseGenericTypeAnnot(embName); isGen {
				lookupName = base
				embTypeArgs = make([]string, len(args))
				substMap := make(map[string]string)
				for i, tp := range sDef.TypeParams {
					if i < len(sDef.TypeArgs) {
						substMap[tp.Name] = sDef.TypeArgs[i]
					}
				}
				for i, a := range args {
					embTypeArgs[i] = substituteTypeString(a, substMap)
				}
			}
			if embDefVal, ok := env.Get(lookupName); ok {
				if embDef, ok := Force(embDefVal).(*StructDefValue); ok {
					targetDef := embDef
					if len(embTypeArgs) > 0 {
						if sp, err := specializeGenericValue(embDef, embTypeArgs, env); err == nil {
							if spDef, ok := sp.(*StructDefValue); ok {
								targetDef = spDef
							}
						}
					}
					subInst, err := env.instantiateStruct(targetDef, []ast.Expr{})
					if err == nil {
						embedded[lookupName] = subInst
						fields[lookupName] = subInst
					}
				}
			}
		}
	}

	inst := &StructInstance{
		Def:      sDef,
		TypeArgs: append([]string(nil), sDef.TypeArgs...),
		Fields:   fields,
		Embedded: embedded,
	}
	if len(inst.TypeArgs) == 0 && len(sDef.TypeParams) > 0 {
		inferStructInstanceTypeArgs(inst)
	}
	// Re-specialize embedded generic structs if type args were inferred
	if len(inst.TypeArgs) > 0 && len(sDef.TypeParams) > 0 {
		substMap := make(map[string]string)
		for i, tp := range sDef.TypeParams {
			if i < len(inst.TypeArgs) {
				substMap[tp.Name] = inst.TypeArgs[i]
			}
		}
		for _, embName := range sDef.EmbeddedNames {
			if base, args, isGen := parseGenericTypeAnnot(embName); isGen {
				lookupName := base
				embTypeArgs := make([]string, len(args))
				for i, a := range args {
					embTypeArgs[i] = substituteTypeString(a, substMap)
				}
				if subInst, ok := inst.Embedded[lookupName]; ok && subInst != nil {
					subInst.TypeArgs = embTypeArgs
				}
			}
		}
	}
	return inst, nil
}

func (env *Environment) instantiateStructWithInit(sDef *StructDefValue, initDef *ast.FuncDef, args []ast.Expr) (*StructInstance, error) {
	fields := make(map[string]Value)
	embedded := make(map[string]*StructInstance)

	isEmbeddedField := make(map[string]bool)
	for _, embName := range sDef.EmbeddedNames {
		isEmbeddedField[embName] = true
		if base, _, isGen := parseGenericTypeAnnot(embName); isGen {
			isEmbeddedField[base] = true
		}
	}

	// 1. Initialize fields with default values
	for _, fName := range sDef.FieldOrder {
		if isEmbeddedField[fName] {
			continue
		}
		if defExpr, hasDef := sDef.FieldDefaults[fName]; hasDef {
			fields[fName] = NewThunk(defExpr, sDef.Env)
		} else {
			fields[fName] = &TupleValue{Elements: []Value{}}
		}
	}

	// 2. Auto-instantiate embedded struct instances if defined in environment
	for _, embName := range sDef.EmbeddedNames {
		lookupName := embName
		var embTypeArgs []string
		if base, args, isGen := parseGenericTypeAnnot(embName); isGen {
			lookupName = base
			embTypeArgs = make([]string, len(args))
			substMap := make(map[string]string)
			for i, tp := range sDef.TypeParams {
				if i < len(sDef.TypeArgs) {
					substMap[tp.Name] = sDef.TypeArgs[i]
				}
			}
			for i, a := range args {
				embTypeArgs[i] = substituteTypeString(a, substMap)
			}
		}
		if embDefVal, ok := env.Get(lookupName); ok {
			if embDef, ok := Force(embDefVal).(*StructDefValue); ok {
				targetDef := embDef
				if len(embTypeArgs) > 0 {
					if sp, err := specializeGenericValue(embDef, embTypeArgs, env); err == nil {
						if spDef, ok := sp.(*StructDefValue); ok {
							targetDef = spDef
						}
					}
				}
				subInst, err := env.instantiateStruct(targetDef, []ast.Expr{})
				if err == nil {
					embedded[lookupName] = subInst
					fields[lookupName] = subInst
				}
			}
		}
	}

	// 3. Create initializing instance
	inst := &StructInstance{
		Def:            sDef,
		TypeArgs:       append([]string(nil), sDef.TypeArgs...),
		Fields:         fields,
		Embedded:       embedded,
		IsInitializing: true,
	}

	// 4. Determine init parameter list (skip explicit 'self' if present)
	initParams := initDef.Params
	if len(initParams) > 0 && initParams[0].Name == "self" {
		initParams = initParams[1:]
	}

	// 5. Match arguments to init parameters and perform auto-binding for matching fields
	argValues := make([]Value, len(initParams))
	posIdx := 0

	namedArgs := make(map[string]ast.Expr)
	var posArgs []ast.Expr
	for _, arg := range args {
		if named, ok := arg.(*ast.NamedArgExpr); ok {
			namedArgs[named.Name] = named.Value
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	for i, param := range initParams {
		if param.IsVariadic {
			var rest []Value
			for p := posIdx; p < len(posArgs); p++ {
				rest = append(rest, env.newThunk(posArgs[p]))
			}
			tup := &TupleValue{Elements: rest}
			argValues[i] = tup
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = tup
					break
				}
			}
			break
		}

		var argExpr ast.Expr
		if expr, ok := namedArgs[param.Name]; ok {
			argExpr = expr
		} else if posIdx < len(posArgs) {
			argExpr = posArgs[posIdx]
			posIdx++
		}

		if argExpr != nil {
			var val Value
			if env.isForceDemand(argExpr) {
				val = Force(env.newThunk(argExpr))
			} else if lit, ok := evalLiteral(argExpr); ok {
				val = lit
			} else {
				val = env.newThunk(argExpr)
			}
			argValues[i] = val

			// Auto-bind: if param name matches struct field, assign to fields
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = val
					break
				}
			}
		} else if param.DefaultValue != nil {
			val := env.newThunk(param.DefaultValue)
			argValues[i] = val
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = val
					break
				}
			}
		} else {
			argValues[i] = &TupleValue{Elements: []Value{}}
		}
	}

	// Also check if any named args directly target fields that weren't in initParams
	for name, expr := range namedArgs {
		foundInParams := false
		for _, param := range initParams {
			if param.Name == name {
				foundInParams = true
				break
			}
		}
		if !foundInParams {
			for _, fName := range sDef.FieldOrder {
				if fName == name {
					fields[name] = env.newThunk(expr)
					break
				}
			}
		}
	}

	// 6. Bind and invoke init
	closure := inst.bindMethod(initDef)
	_, err := env.invokeClosure(closure, argValues)
	if err != nil {
		return nil, err
	}

	// 7. Freeze instance
	inst.IsInitializing = false
	if len(inst.TypeArgs) == 0 && len(sDef.TypeParams) > 0 {
		inferStructInstanceTypeArgs(inst)
	}
	// Re-specialize embedded generic structs if type args were inferred
	if len(inst.TypeArgs) > 0 && len(sDef.TypeParams) > 0 {
		substMap := make(map[string]string)
		for i, tp := range sDef.TypeParams {
			if i < len(inst.TypeArgs) {
				substMap[tp.Name] = inst.TypeArgs[i]
			}
		}
		for _, embName := range sDef.EmbeddedNames {
			if base, args, isGen := parseGenericTypeAnnot(embName); isGen {
				lookupName := base
				embTypeArgs := make([]string, len(args))
				for i, a := range args {
					embTypeArgs[i] = substituteTypeString(a, substMap)
				}
				if subInst, ok := inst.Embedded[lookupName]; ok && subInst != nil {
					subInst.TypeArgs = embTypeArgs
				}
			}
		}
	}
	return inst, nil
}

type eqPair struct {
	p1 any
	p2 any
}

func evalBinary(op string, left, right Value) (Value, error) {
	return evalBinaryWithVisited(op, left, right, make(map[eqPair]bool), 0)
}

func evalBinaryWithVisited(op string, left, right Value, visitedPairs map[eqPair]bool, depth int) (Value, error) {
	// Nil comparison
	_, leftIsNil := left.(NilValue)
	_, rightIsNil := right.(NilValue)
	if leftIsNil || rightIsNil {
		if op == "==" {
			return BoolValue(leftIsNil && rightIsNil), nil
		}
		if op == "!=" {
			return BoolValue(leftIsNil != rightIsNil), nil
		}
	}

	// != is always desugared to !(x == y)
	if op == "!=" {
		eqVal, err := evalBinaryWithVisited("==", left, right, visitedPairs, depth)
		if err != nil {
			return nil, err
		}
		if bVal, ok := eqVal.(BoolValue); ok {
			return !bVal, nil
		}
		return nil, fmt.Errorf("== operator must return Bool, got %v", eqVal)
	}

	// Tuple operators (+, <<, ==, !=)
	if lt, ok := left.(*TupleValue); ok {
		switch op {
		case "+":
			rt, ok := right.(*TupleValue)
			if !ok {
				return nil, fmt.Errorf("cannot concatenate tuple and %s", typeNameOf(right))
			}
			newElems := append(append([]Value(nil), lt.Elements...), rt.Elements...)
			return &TupleValue{Elements: newElems, IsVariadic: lt.IsVariadic || rt.IsVariadic}, nil
		case "<<":
			newElems := append(append([]Value(nil), lt.Elements...), right)
			return &TupleValue{Elements: newElems, IsVariadic: lt.IsVariadic}, nil
		case "==":
			rt, ok := right.(*TupleValue)
			if !ok || len(lt.Elements) != len(rt.Elements) || lt.IsVariadic != rt.IsVariadic {
				return BoolValue(false), nil
			}
			if lt == rt {
				return BoolValue(true), nil
			}
			pair := eqPair{p1: lt, p2: rt}
			revPair := eqPair{p1: rt, p2: lt}
			if visitedPairs[pair] || visitedPairs[revPair] || depth > maxInspectDepth {
				return BoolValue(true), nil
			}
			visitedPairs[pair] = true
			defer delete(visitedPairs, pair)

			for i := range lt.Elements {
				eqVal, err := evalBinaryWithVisited("==", Force(lt.Elements[i]), Force(rt.Elements[i]), visitedPairs, depth+1)
				if err != nil {
					return nil, err
				}
				if b, ok := eqVal.(BoolValue); !ok || !bool(b) {
					return BoolValue(false), nil
				}
			}
			return BoolValue(true), nil
		}
	}

	// Custom operator methods on StructInstance
	if si, ok := left.(*StructInstance); ok {
		if fn, err := si.LookupMethod(op, 1); err == nil {
			return fn.Env.invokeClosure(fn, []Value{right})
		}
		if m, err := si.LookupMember(op); err == nil {
			forced := Force(m)
			if fn, ok := forced.(*Closure); ok {
				return fn.Env.invokeClosure(fn, []Value{right})
			}
			if bf, ok := forced.(*BuiltinFunc); ok {
				return bf.Fn([]Value{right})
			}
		}
		// Relational comparison fallback using < and == (Comparable)
		if _, hasLt := si.LookupMethod("<", 1); hasLt == nil {
			switch op {
			case "<=":
				// left <= right <=> (left < right) || (left == right)
				ltVal, errLt := evalBinary("<", left, right)
				if errLt == nil {
					if b, ok := ltVal.(BoolValue); ok && bool(b) {
						return BoolValue(true), nil
					}
					eqVal, errEq := evalBinary("==", left, right)
					if errEq == nil {
						if b, ok := eqVal.(BoolValue); ok {
							return b, nil
						}
					}
				}
			case ">":
				// left > right <=> right < left
				return evalBinary("<", right, left)
			case ">=":
				// left >= right <=> right <= left
				return evalBinary("<=", right, left)
			}
		}
	}

	// Numeric operations
	if li, ok := left.(IntValue); ok {
		if ri, ok := right.(IntValue); ok {
			switch op {
			case "+":
				return li.Add(ri), nil
			case "-":
				return li.Sub(ri), nil
			case "*":
				return li.Mul(ri), nil
			case "/":
				return li.Quo(ri)
			case "%":
				return li.Rem(ri)
			case "&":
				return li.And(ri), nil
			case "|":
				return li.Or(ri), nil
			case "^":
				return li.Xor(ri), nil
			case "<<":
				return li.Lsh(ri)
			case ">>":
				return li.Rsh(ri)
			case "==":
				return BoolValue(li.Equal(ri)), nil
			case "!=":
				return BoolValue(!li.Equal(ri)), nil
			case "<":
				return BoolValue(li.Cmp(ri) < 0), nil
			case "<=":
				return BoolValue(li.Cmp(ri) <= 0), nil
			case ">":
				return BoolValue(li.Cmp(ri) > 0), nil
			case ">=":
				return BoolValue(li.Cmp(ri) >= 0), nil
			}
		}
	}

	// Float operations
	if lf, ok := left.(FloatValue); ok {
		if rf, ok := right.(FloatValue); ok {
			switch op {
			case "+":
				return lf + rf, nil
			case "-":
				return lf - rf, nil
			case "*":
				return lf * rf, nil
			case "/":
				if rf == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				return lf / rf, nil
			case "==":
				return BoolValue(lf == rf), nil
			case "!=":
				return BoolValue(lf != rf), nil
			case "<":
				return BoolValue(lf < rf), nil
			case "<=":
				return BoolValue(lf <= rf), nil
			case ">":
				return BoolValue(lf > rf), nil
			case ">=":
				return BoolValue(lf >= rf), nil
			}
		}
	}

	// String concatenation & comparison
	if ls, ok := left.(StringValue); ok {
		if rs, ok := right.(StringValue); ok {
			switch op {
			case "+":
				return ls + rs, nil
			case "==":
				return BoolValue(ls == rs), nil
			case "!=":
				return BoolValue(ls != rs), nil
			case "<":
				return BoolValue(ls < rs), nil
			case "<=":
				return BoolValue(ls <= rs), nil
			case ">":
				return BoolValue(ls > rs), nil
			case ">=":
				return BoolValue(ls >= rs), nil
			}
		}
	}

	// Bytes concatenation & comparison
	if lb, ok := left.(BytesValue); ok {
		if rb, ok := right.(BytesValue); ok {
			switch op {
			case "+":
				return lb + rb, nil
			case "==":
				return BoolValue(lb == rb), nil
			case "!=":
				return BoolValue(lb != rb), nil
			case "<":
				return BoolValue(lb < rb), nil
			case "<=":
				return BoolValue(lb <= rb), nil
			case ">":
				return BoolValue(lb > rb), nil
			case ">=":
				return BoolValue(lb >= rb), nil
			}
		}
	}

	// Boolean operations
	if lb, ok := left.(BoolValue); ok {
		if rb, ok := right.(BoolValue); ok {
			switch op {
			case "&&":
				return BoolValue(bool(lb) && bool(rb)), nil
			case "||":
				return BoolValue(bool(lb) || bool(rb)), nil
			case "==":
				return BoolValue(lb == rb), nil
			case "!=":
				return BoolValue(lb != rb), nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported binary operation %v %s %v", left, op, right)
}

func inspectString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func lookupBuiltinMember(val Value, member string) (Value, error) {
	if member == "Inspect" {
		return &BuiltinFunc{
			Name: "Inspect",
			FuncType: &types.FuncType{
				Arg: &types.TupleType{Elems: []types.Type{}},
				Ret: types.TypeString,
			},
			Fn: func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("Inspect() takes 0 arguments, got %d", len(args))
				}
				return StringValue(InspectValue(val)), nil
			},
		}, nil
	}
	if member == "Type" {
		return &BuiltinFunc{
			Name: "Type",
			FuncType: &types.FuncType{
				Arg: &types.TupleType{Elems: []types.Type{}},
				Ret: types.TypeString,
			},
			Fn: func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("Type() takes 0 arguments, got %d", len(args))
				}
				return StringValue(typeNameOf(val)), nil
			},
		}, nil
	}
	if member == "Len" {
		switch v := val.(type) {
		case StringValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return NewIntValue(int64(len([]rune(string(v))))), nil
				},
			}, nil
		case BytesValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return NewIntValue(int64(len(v))), nil
				},
			}, nil
		case *TupleValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					if v.IsVariadic {
						return NewIntValue(int64(-(len(v.Elements) + 1))), nil
					}
					return NewIntValue(int64(len(v.Elements))), nil
				},
			}, nil
		case *DictValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return NewIntValue(int64(len(v.Entries))), nil
				},
			}, nil
		}
	}
	if member == "Ord" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Ord",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Ord() takes 0 arguments, got %d", len(args))
					}
					s := string(sv)
					if len(s) == 0 {
						return nil, fmt.Errorf("Ord() on empty string")
					}
					r, size := utf8.DecodeRuneInString(s)
					if r == utf8.RuneError && size <= 1 {
						return nil, fmt.Errorf("Ord() invalid UTF-8 string")
					}
					return NewIntValue(int64(r)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Ord"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Chr" {
		if iv, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: "Chr",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Chr() takes 0 arguments, got %d", len(args))
					}
					n, ok := iv.Int64()
					if !ok || n < 0 || n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
						return nil, fmt.Errorf("Chr() code point %s out of valid Unicode range", iv.String())
					}
					return StringValue(string(rune(n))), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, "Chr"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Bytes" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Bytes",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Bytes() takes 0 arguments, got %d", len(args))
					}
					return BytesValue(string(sv)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Bytes"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "Bytes",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Bytes() takes 0 arguments, got %d", len(args))
					}
					return bv, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "Bytes"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "String" {
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "String",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("String() takes 0 arguments, got %d", len(args))
					}
					b := []byte(bv)
					if !utf8.Valid(b) {
						return nil, fmt.Errorf("invalid UTF-8 byte sequence in Bytes.String()")
					}
					return StringValue(string(bv)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "String"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
		if iv, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: "String",
				Fn: func(args []Value) (Value, error) {
					radix := 10
					if len(args) == 1 {
						rVal := Force(args[0])
						rv, ok := rVal.(IntValue)
						if !ok {
							return nil, fmt.Errorf("String() radix must be an integer, got %s", typeNameOf(rVal))
						}
						rad, ok := rv.ToInt()
						if !ok {
							return nil, fmt.Errorf("unsupported radix %s (supported: 2, 8, 10, 16)", rv.String())
						}
						radix = rad
					} else if len(args) > 1 {
						return nil, fmt.Errorf("String() takes at most 1 argument, got %d", len(args))
					}

					if radix != 2 && radix != 8 && radix != 10 && radix != 16 {
						return nil, fmt.Errorf("unsupported radix %d (supported: 2, 8, 10, 16)", radix)
					}

					formatted, err := iv.Format(radix)
					if err != nil {
						return nil, err
					}
					return StringValue(formatted), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, "String"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Int" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Int",
				Fn: func(args []Value) (Value, error) {
					radix := 10
					if len(args) == 1 {
						rVal := Force(args[0])
						iv, ok := rVal.(IntValue)
						if !ok {
							return nil, fmt.Errorf("Int() radix must be an integer, got %s", typeNameOf(rVal))
						}
						rad, ok := iv.ToInt()
						if !ok {
							return nil, fmt.Errorf("unsupported radix %s (supported: 2, 8, 10, 16)", iv.String())
						}
						radix = rad
					} else if len(args) > 1 {
						return nil, fmt.Errorf("Int() takes at most 1 argument, got %d", len(args))
					}

					if radix != 2 && radix != 8 && radix != 10 && radix != 16 {
						return nil, fmt.Errorf("unsupported radix %d (supported: 2, 8, 10, 16)", radix)
					}

					return parseStringToInt(string(sv), radix), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Int"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "IsValidString" {
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "IsValidString",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("IsValidString() takes 0 arguments, got %d", len(args))
					}
					return BoolValue(utf8.Valid([]byte(bv))), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "IsValidString"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}

	visitedTypes := make(map[any]bool)
	var typeOfValWithDepth func(v Value, depth int) types.Type
	typeOfValWithDepth = func(v Value, depth int) types.Type {
		if thunk, ok := v.(*Thunk); ok {
			if evaluated, isEval := PeekEvaluated(thunk); isEval {
				v = evaluated
			} else if thunk.InferredType != nil {
				if t, ok := thunk.InferredType.(types.Type); ok && t != nil && t != types.TypeUnknown {
					return t
				}
			}
		}
		switch val := v.(type) {
		case IntValue:
			return types.TypeInt
		case FloatValue:
			return types.TypeFloat
		case StringValue:
			return types.TypeString
		case BytesValue:
			return types.TypeBytes
		case BoolValue:
			return types.TypeBool
		case NilValue:
			return types.TypeNil
		case *TupleValue:
			if visitedTypes[val] || depth > 20 {
				return &types.TupleType{Elems: []types.Type{}}
			}
			visitedTypes[val] = true
			defer delete(visitedTypes, val)
			elems := make([]types.Type, len(val.Elements))
			for i, el := range val.Elements {
				elems[i] = typeOfValWithDepth(el, depth+1)
			}
			return &types.TupleType{Elems: elems, IsVariadic: val.IsVariadic}
		case *DictValue:
			if visitedTypes[val] || depth > 20 {
				return &types.DictType{Key: types.TypeUnknown, Value: types.TypeUnknown}
			}
			visitedTypes[val] = true
			defer delete(visitedTypes, val)
			var kList []types.Type
			var vList []types.Type
			for dk, v := range val.Entries {
				if origKey, ok := val.KeyValues[dk]; ok {
					kList = append(kList, typeOfValWithDepth(origKey, depth+1))
				}
				vList = append(vList, typeOfValWithDepth(v, depth+1))
			}
			var kT types.Type = types.TypeUnknown
			if len(kList) > 0 {
				kT = types.NewUnionType(kList...)
			}
			var vT types.Type = types.TypeUnknown
			if len(vList) > 0 {
				vT = types.NewUnionType(vList...)
			}
			return &types.DictType{Key: kT, Value: vT}
		default:
			return types.TypeUnknown
		}
	}
	typeOfVal := func(v Value) types.Type {
		return typeOfValWithDepth(v, 0)
	}

	// Operator methods on built-in types
	switch member {
	case "==":
		switch val.(type) {
		case IntValue, FloatValue, StringValue, BytesValue, BoolValue, NilValue, *TupleValue, *DictValue:
			bf := &BuiltinFunc{
				Name: "==",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("== takes 1 argument, got %d", len(args))
					}
					return evalBinary("==", val, Force(args[0]))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "=="); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "<", "<=", ">", ">=":
		switch val.(type) {
		case IntValue, FloatValue, StringValue, BytesValue:
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("%s takes 1 argument, got %d", member, len(args))
					}
					return evalBinary(member, val, Force(args[0]))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), member); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "+", "-":
		switch val.(type) {
		case IntValue, FloatValue:
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) == 0 {
						if member == "+" {
							return val, nil
						}
						if iv, ok := val.(IntValue); ok {
							return iv.Neg(), nil
						}
						if fv, ok := val.(FloatValue); ok {
							return -fv, nil
						}
					} else if len(args) == 1 {
						return evalBinary(member, val, Force(args[0]))
					}
					return nil, fmt.Errorf("%s takes 0 or 1 argument, got %d", member, len(args))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), member); ok {
				bf.FuncType = sig
			}
			return bf, nil
		case StringValue, BytesValue:
			if member == "+" {
				bf := &BuiltinFunc{
					Name: "+",
					Fn: func(args []Value) (Value, error) {
						if len(args) != 1 {
							return nil, fmt.Errorf("+ takes 1 argument, got %d", len(args))
						}
						return evalBinary("+", val, Force(args[0]))
					},
				}
				if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "+"); ok {
					bf.FuncType = sig
				}
				return bf, nil
			}
		case *TupleValue:
			if member == "+" {
				bf := &BuiltinFunc{
					Name: "+",
					Fn: func(args []Value) (Value, error) {
						if len(args) != 1 {
							return nil, fmt.Errorf("+ takes 1 argument, got %d", len(args))
						}
						return evalBinary("+", val, Force(args[0]))
					},
				}
				if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "+"); ok {
					bf.FuncType = sig
				}
				return bf, nil
			}
		}

	case "*", "/", "%":
		switch val.(type) {
		case IntValue:
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("%s takes 1 argument, got %d", member, len(args))
					}
					return evalBinary(member, val, Force(args[0]))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, member); ok {
				bf.FuncType = sig
			}
			return bf, nil
		case FloatValue:
			if member == "*" || member == "/" {
				bf := &BuiltinFunc{
					Name: member,
					Fn: func(args []Value) (Value, error) {
						if len(args) != 1 {
							return nil, fmt.Errorf("%s takes 1 argument, got %d", member, len(args))
						}
						return evalBinary(member, val, Force(args[0]))
					},
				}
				if sig, ok := types.LookupBuiltinTypeMethod(types.TypeFloat, member); ok {
					bf.FuncType = sig
				}
				return bf, nil
			}
		}

	case "~":
		if iv, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: "~",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("~ takes 0 arguments, got %d", len(args))
					}
					return iv.Not(), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, "~"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "&", "|", "^", ">>":
		if _, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("%s takes 1 argument, got %d", member, len(args))
					}
					return evalBinary(member, val, Force(args[0]))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, member); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "<<":
		if _, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("%s takes 1 argument, got %d", member, len(args))
					}
					return evalBinary(member, val, Force(args[0]))
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, member); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
		if tt, ok := val.(*TupleValue); ok {
			bf := &BuiltinFunc{
				Name: "<<",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("<< takes 1 argument, got %d", len(args))
					}
					newElems := append(append([]Value(nil), tt.Elements...), args[0])
					return &TupleValue{Elements: newElems, IsVariadic: tt.IsVariadic}, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "<<"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "Append":
		if tt, ok := val.(*TupleValue); ok {
			bf := &BuiltinFunc{
				Name: member,
				Fn: func(args []Value) (Value, error) {
					if len(args) == 0 {
						return nil, fmt.Errorf("%s takes at least 1 argument, got 0", member)
					}
					var elemToAppend Value
					if len(args) == 1 {
						elemToAppend = args[0]
					} else {
						elemToAppend = &TupleValue{Elements: append([]Value(nil), args...)}
					}
					newElems := append(append([]Value(nil), tt.Elements...), elemToAppend)
					return &TupleValue{Elements: newElems, IsVariadic: tt.IsVariadic}, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "Append"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "Prepend":
		if tt, ok := val.(*TupleValue); ok {
			bf := &BuiltinFunc{
				Name: "Prepend",
				Fn: func(args []Value) (Value, error) {
					if len(args) == 0 {
						return nil, fmt.Errorf("Prepend takes at least 1 argument, got 0")
					}
					var elemToPrepend Value
					if len(args) == 1 {
						elemToPrepend = args[0]
					} else {
						elemToPrepend = &TupleValue{Elements: append([]Value(nil), args...)}
					}
					newElems := append([]Value{elemToPrepend}, tt.Elements...)
					return &TupleValue{Elements: newElems, IsVariadic: tt.IsVariadic}, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "Prepend"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "Concat":
		if tt, ok := val.(*TupleValue); ok {
			bf := &BuiltinFunc{
				Name: "Concat",
				Fn: func(args []Value) (Value, error) {
					if len(args) < 1 {
						return nil, fmt.Errorf("Concat takes at least 1 argument, got 0")
					}
					newElems := append([]Value(nil), tt.Elements...)
					isVariadic := tt.IsVariadic
					for i, arg := range args {
						forced := Force(arg)
						otherT, ok := forced.(*TupleValue)
						if !ok {
							return nil, fmt.Errorf("Concat argument %d must be a tuple, got %s", i+1, typeNameOf(forced))
						}
						newElems = append(newElems, otherT.Elements...)
						if otherT.IsVariadic {
							isVariadic = true
						}
					}
					return &TupleValue{Elements: newElems, IsVariadic: isVariadic}, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "Concat"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "Slice":
		if tt, ok := val.(*TupleValue); ok {
			bf := &BuiltinFunc{
				Name: "Slice",
				Fn: func(args []Value) (Value, error) {
					if len(args) < 1 || len(args) > 2 {
						return nil, fmt.Errorf("Slice takes 1 or 2 arguments, got %d", len(args))
					}
					startVal := Force(args[0])
					sInt, ok := startVal.(IntValue)
					if !ok {
						return nil, fmt.Errorf("Slice start index must be an integer, got %s", typeNameOf(startVal))
					}
					if tt.IsVariadic && sInt.Cmp(NewIntValue(0)) < 0 {
						return nil, fmt.Errorf("cannot use negative index on infinite tuple: %s", sInt.String())
					}
					var endVal Value
					var eInt IntValue
					if len(args) == 2 {
						endVal = Force(args[1])
						ei, ok := endVal.(IntValue)
						if !ok {
							return nil, fmt.Errorf("Slice end index must be an integer, got %s", typeNameOf(endVal))
						}
						eInt = ei
						if tt.IsVariadic && eInt.Cmp(NewIntValue(0)) < 0 {
							return nil, fmt.Errorf("cannot use negative index on infinite tuple: %s", eInt.String())
						}
					}

					if tt.IsVariadic {
						sIdx64, ok := sInt.ToInt()
						if !ok {
							if sInt.Cmp(NewIntValue(0)) < 0 {
								sIdx64 = 0
							} else {
								sIdx64 = len(tt.Elements)
							}
						}
						sIdx := sIdx64
						if sIdx < 0 {
							sIdx = 0
						}
						if len(args) == 1 {
							var newElems []Value
							if sIdx < len(tt.Elements) {
								newElems = append([]Value(nil), tt.Elements[sIdx:]...)
							} else if len(tt.Elements) > 0 {
								newElems = []Value{tt.Elements[len(tt.Elements)-1]}
							}
							return &TupleValue{Elements: newElems, IsVariadic: true}, nil
						}
						eIdx64, ok := eInt.ToInt()
						if !ok {
							if eInt.Cmp(NewIntValue(0)) < 0 {
								eIdx64 = 0
							} else {
								eIdx64 = sIdx
							}
						}
						eIdx := eIdx64
						if eIdx < sIdx {
							eIdx = sIdx
						}
						var newElems []Value
						for i := sIdx; i < eIdx; i++ {
							if i < len(tt.Elements) {
								newElems = append(newElems, tt.Elements[i])
							} else if len(tt.Elements) > 0 {
								newElems = append(newElems, tt.Elements[len(tt.Elements)-1])
							}
						}
						return &TupleValue{Elements: newElems, IsVariadic: false}, nil
					}

					n := len(tt.Elements)
					sIdx64, ok := sInt.ToInt()
					if !ok {
						if sInt.Cmp(NewIntValue(0)) < 0 {
							sIdx64 = 0
						} else {
							sIdx64 = n
						}
					}
					sIdx := sIdx64
					if sIdx < 0 {
						sIdx = n + sIdx
					}
					if sIdx < 0 {
						sIdx = 0
					}
					if sIdx > n {
						sIdx = n
					}

					eIdx := n
					if len(args) == 2 {
						eIdx64, ok := eInt.ToInt()
						if !ok {
							if eInt.Cmp(NewIntValue(0)) < 0 {
								eIdx64 = 0
							} else {
								eIdx64 = n
							}
						}
						eIdx = eIdx64
						if eIdx < 0 {
							eIdx = n + eIdx
						}
						if eIdx < 0 {
							eIdx = 0
						}
						if eIdx > n {
							eIdx = n
						}
					}
					if eIdx < sIdx {
						eIdx = sIdx
					}
					return &TupleValue{Elements: append([]Value(nil), tt.Elements[sIdx:eIdx]...)}, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "Slice"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}

	case "[]":
		switch t := val.(type) {
		case StringValue:
			bf := &BuiltinFunc{
				Name: "[]",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("[] takes 1 argument, got %d", len(args))
					}
					idxVal := Force(args[0])
					i, ok := idxVal.(IntValue)
					if !ok {
						return nil, fmt.Errorf("string index must be Int, got %v", idxVal)
					}
					runes := []rune(string(t))
					idx, ok := i.ToInt()
					if !ok {
						return nil, fmt.Errorf("string index out of bounds: %s (len %d)", i.String(), len(runes))
					}
					if idx < 0 {
						idx = len(runes) + idx
					}
					if idx < 0 || idx >= len(runes) {
						return nil, fmt.Errorf("string index out of bounds: %s (len %d)", i.String(), len(runes))
					}
					return StringValue(string(runes[idx])), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "[]"); ok {
				bf.FuncType = sig
			}
			return bf, nil

		case BytesValue:
			bf := &BuiltinFunc{
				Name: "[]",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("[] takes 1 argument, got %d", len(args))
					}
					idxVal := Force(args[0])
					i, ok := idxVal.(IntValue)
					if !ok {
						return nil, fmt.Errorf("bytes index must be Int, got %v", idxVal)
					}
					data := []byte(t)
					idx, ok := i.ToInt()
					if !ok {
						return nil, fmt.Errorf("bytes index out of bounds: %s (len %d)", i.String(), len(data))
					}
					if idx < 0 {
						idx = len(data) + idx
					}
					if idx < 0 || idx >= len(data) {
						return nil, fmt.Errorf("bytes index out of bounds: %s (len %d)", i.String(), len(data))
					}
					return NewIntValue(int64(data[idx])), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "[]"); ok {
				bf.FuncType = sig
			}
			return bf, nil

		case *TupleValue:
			bf := &BuiltinFunc{
				Name: "[]",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("[] takes 1 argument, got %d", len(args))
					}
					idxVal := Force(args[0])
					i, ok := idxVal.(IntValue)
					if !ok {
						return nil, fmt.Errorf("tuple index must be Int, got %v", idxVal)
					}
					idx, ok := i.ToInt()
					if !ok {
						return nil, fmt.Errorf("tuple index out of bounds: %s (len %d)", i.String(), len(t.Elements))
					}
					if idx < 0 {
						if t.IsVariadic {
							return nil, fmt.Errorf("cannot use negative index on infinite tuple: %s", i.String())
						}
						idx = len(t.Elements) + idx
					}
					if idx < 0 || idx >= len(t.Elements) {
						if t.IsVariadic && len(t.Elements) > 0 && idx >= len(t.Elements) {
							return Force(t.Elements[len(t.Elements)-1]), nil
						}
						return nil, fmt.Errorf("tuple index out of bounds: %s (len %d)", i.String(), len(t.Elements))
					}
					return Force(t.Elements[idx]), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "[]"); ok {
				bf.FuncType = sig
			} else {
				bf.FuncType = &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{types.TypeInt}},
					Ret: types.TypeUnknown,
				}
			}
			return bf, nil

		case *DictValue:
			bf := &BuiltinFunc{
				Name: "[]",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("[] takes 1 argument, got %d", len(args))
					}
					idxVal := Force(args[0])
					dk := MakeDictKey(idxVal)
					if v, ok := t.Entries[dk]; ok {
						return Force(v), nil
					}
					return nil, fmt.Errorf("key not found in dict: %s", idxVal.String())
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(typeOfVal(val), "[]"); ok {
				bf.FuncType = sig
			} else {
				bf.FuncType = &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{types.TypeUnknown}},
					Ret: types.TypeUnknown,
				}
			}
			return bf, nil
		}
	}

	return nil, fmt.Errorf("cannot access member '%s' on %s value: %v", member, typeNameOf(val), val)
}

func isValidDigitForRadix(c byte, radix int) bool {
	switch radix {
	case 2:
		return c == '0' || c == '1'
	case 8:
		return c >= '0' && c <= '7'
	case 10:
		return c >= '0' && c <= '9'
	case 16:
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	default:
		return false
	}
}

func parseStringToInt(s string, radix int) IntValue {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return NewIntValue(0)
	}
	start := 0
	if s[0] == '+' || s[0] == '-' {
		start = 1
	}
	end := start
	for end < len(s) {
		c := s[end]
		if !isValidDigitForRadix(c, radix) {
			break
		}
		end++
	}
	if end == start {
		return NewIntValue(0)
	}
	sub := s[:end]
	v, err := strconv.ParseInt(sub, radix, 64)
	if err != nil {
		if strings.Contains(err.Error(), "value out of range") {
			z, ok := new(big.Int).SetString(sub, radix)
			if ok {
				return NewIntValueFromBig(z)
			}
		}
		return NewIntValue(0)
	}
	return NewIntValue(v)
}

func checkValueMatchesType(val Value, typeAnnot string, optEnv ...*Environment) bool {
	typeAnnot = strings.TrimSpace(typeAnnot)
	if typeAnnot == "" {
		return true
	}
	if thunk, ok := val.(*Thunk); ok {
		if evaluated, isEval := PeekEvaluated(thunk); isEval {
			val = evaluated
		} else {
			return checkThunkInferredType(thunk, typeAnnot, optEnv...)
		}
	}
	// Check top-level union: A | B (respecting parens, brackets, braces)
	var unionParts []string
	pDepth := 0
	bDepth := 0
	brDepth := 0
	start := 0
	runes := []rune(typeAnnot)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '(' {
			pDepth++
		} else if r == ')' {
			pDepth--
		} else if r == '[' {
			bDepth++
		} else if r == ']' {
			bDepth--
		} else if r == '{' {
			brDepth++
		} else if r == '}' {
			brDepth--
		} else if r == '|' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
			unionParts = append(unionParts, string(runes[start:i]))
			start = i + 1
		}
	}
	if len(unionParts) > 0 {
		unionParts = append(unionParts, string(runes[start:]))
		for _, p := range unionParts {
			if checkValueMatchesType(val, strings.TrimSpace(p), optEnv...) {
				return true
			}
		}
		return false
	}

	switch typeAnnot {
	case "Nil", "nil":
		_, ok := val.(NilValue)
		return ok
	case "true":
		b, ok := val.(BoolValue)
		return ok && bool(b)
	case "false":
		b, ok := val.(BoolValue)
		return ok && !bool(b)
	case "Int":
		_, ok := val.(IntValue)
		return ok
	case "Float":
		_, ok := val.(FloatValue)
		return ok
	case "String":
		_, ok := val.(StringValue)
		return ok
	case "Bytes":
		_, ok := val.(BytesValue)
		return ok
	case "Bool":
		_, ok := val.(BoolValue)
		return ok
	case "Tuple":
		_, ok := val.(*TupleValue)
		return ok
	case "Dict":
		_, ok := val.(*DictValue)
		return ok
	case "Function":
		switch val.(type) {
		case *Closure, *BuiltinFunc, *StructDefValue:
			return true
		default:
			return false
		}
	default:
		switch fnVal := val.(type) {
		case *Closure:
			if fnT, ok := fnVal.FuncType.(*types.FuncType); ok {
				var targetType types.Type
				if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
					if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
						targetType, _ = inf.ParseTypeAnnot(typeAnnot)
					}
				}
				if targetType == nil {
					targetType, _ = types.ParseTypeAnnot(typeAnnot)
				}
				if targetFT, ok := targetType.(*types.FuncType); ok {
					return types.IsAssignableTo(fnT, targetFT)
				}
			}
		case *BuiltinFunc:
			if fnT, ok := fnVal.FuncType.(*types.FuncType); ok {
				var targetType types.Type
				if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
					if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
						targetType, _ = inf.ParseTypeAnnot(typeAnnot)
					}
				}
				if targetType == nil {
					targetType, _ = types.ParseTypeAnnot(typeAnnot)
				}
				if targetFT, ok := targetType.(*types.FuncType); ok {
					return types.IsAssignableTo(fnT, targetFT)
				}
			}
		case *StructDefValue:
			if st, ok := fnVal.StructType.(*types.StructType); ok && st.Methods != nil {
				if fnT, exists := st.Methods["init"]; exists && fnT != nil {
					var targetType types.Type
					if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
						if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
							targetType, _ = inf.ParseTypeAnnot(typeAnnot)
						}
					}
					if targetType == nil {
						targetType, _ = types.ParseTypeAnnot(typeAnnot)
					}
					if targetFT, ok := targetType.(*types.FuncType); ok {
						return types.IsAssignableTo(fnT, targetFT)
					}
				}
			}
		}
		if strings.HasPrefix(typeAnnot, "{") && strings.HasSuffix(typeAnnot, "}") {
			dVal, ok := val.(*DictValue)
			if !ok {
				return false
			}
			inner := strings.TrimSpace(typeAnnot[1 : len(typeAnnot)-1])
			if inner == "" {
				return false
			}
			if inner == "..." {
				return true
			}
			colonIdx := -1
			pD := 0
			bD := 0
			brD := 0
			innerRunes := []rune(inner)
			for i, r := range innerRunes {
				if r == '(' {
					pD++
				} else if r == ')' {
					pD--
				} else if r == '[' {
					bD++
				} else if r == ']' {
					bD--
				} else if r == '{' {
					brD++
				} else if r == '}' {
					brD--
				} else if r == ':' && pD == 0 && bD == 0 && brD == 0 {
					if colonIdx != -1 {
						return false
					}
					colonIdx = i
				}
			}
			if colonIdx == -1 {
				return false
			}
			keyAnnot := strings.TrimSpace(string(innerRunes[:colonIdx]))
			valAnnot := strings.TrimSpace(string(innerRunes[colonIdx+1:]))
			if keyAnnot == "" || valAnnot == "" {
				return false
			}

			for dk, v := range dVal.Entries {
				var kVal Value
				if dVal.KeyValues != nil {
					if actualKey, exists := dVal.KeyValues[dk]; exists {
						kVal = actualKey
					}
				}
				if kVal == nil {
					kVal = StringValue(dk.Key)
				}
				if !checkValueMatchesType(kVal, keyAnnot, optEnv...) {
					return false
				}
				// Check value:
				if evaluatedVal, ok := PeekEvaluated(v); ok {
					if !checkValueMatchesType(evaluatedVal, valAnnot, optEnv...) {
						return false
					}
				} else if thunk, ok := v.(*Thunk); ok {
					if !checkThunkInferredType(thunk, valAnnot, optEnv...) {
						return false
					}
				}
			}
			return true
		}
		// Check tuple type annotation: (T1, T2, ...) or (T, ...)
		if strings.HasPrefix(typeAnnot, "(") && strings.HasSuffix(typeAnnot, ")") {
			tVal, ok := val.(*TupleValue)
			if !ok {
				return false
			}
			inner := strings.TrimSpace(typeAnnot[1 : len(typeAnnot)-1])
			if inner == "" {
				return len(tVal.Elements) == 0 && !tVal.IsVariadic
			}
			if inner == "..." {
				return true
			}
			elemAnnots := splitTopLevelCommas(inner)
			isVariadic := false
			if len(elemAnnots) > 0 && elemAnnots[len(elemAnnots)-1] == "..." {
				isVariadic = true
				elemAnnots = elemAnnots[:len(elemAnnots)-1]
			}
			if isVariadic {
				if len(elemAnnots) > 0 {
					requiredPrefix := len(elemAnnots) - 1
					if !tVal.IsVariadic && len(tVal.Elements) < requiredPrefix {
						return false
					}
					if tVal.IsVariadic && len(tVal.Elements) < requiredPrefix {
						return false
					}
					if tVal.IsVariadic && len(tVal.Elements) == 0 {
						return false
					}
				}
			} else {
				if tVal.IsVariadic {
					return false
				}
				if len(tVal.Elements) != len(elemAnnots) {
					return false
				}
			}
			checkCount := len(tVal.Elements)
			if isVariadic && tVal.IsVariadic && len(elemAnnots) > checkCount {
				checkCount = len(elemAnnots)
			}
			lastIdx := len(elemAnnots) - 1
			for i := 0; i < checkCount; i++ {
				var el Value
				if i < len(tVal.Elements) {
					el = tVal.Elements[i]
				} else {
					el = tVal.Elements[len(tVal.Elements)-1]
				}
				expectedAnnot := "Unknown"
				if lastIdx >= 0 {
					if i < len(elemAnnots) {
						expectedAnnot = elemAnnots[i]
					} else if isVariadic {
						expectedAnnot = elemAnnots[lastIdx]
					} else {
						return false
					}
				}
				if evaluatedVal, ok := PeekEvaluated(el); ok {
					if !checkValueMatchesType(evaluatedVal, expectedAnnot, optEnv...) {
						return false
					}
				} else if thunk, ok := el.(*Thunk); ok {
					if !checkThunkInferredType(thunk, expectedAnnot, optEnv...) {
						return false
					}
				}
			}
			return true
		}
		// Check generic struct or generic interface type annotation: Name[T1, T2]
		baseName, typeArgs, isGeneric := parseGenericTypeAnnot(typeAnnot)
		if isGeneric {
			if si, ok := val.(*StructInstance); ok {
				if si.Def.Name == baseName {
					if len(si.TypeArgs) == 0 && len(si.Def.TypeParams) > 0 {
						inferStructInstanceTypeArgs(si)
					}
					if len(si.TypeArgs) == len(typeArgs) {
						allMatch := true
						for i, ta := range typeArgs {
							if si.TypeArgs[i] == ta {
								continue
							}
							actualT, err1 := types.ParseTypeAnnot(si.TypeArgs[i])
							targetT, err2 := types.ParseTypeAnnot(ta)
							if err1 == nil && err2 == nil && types.IsAssignableTo(actualT, targetT) {
								continue
							}
							allMatch = false
							break
						}
						if allMatch {
							return true
						}
					}
				}
				for _, emb := range si.Embedded {
					if checkValueMatchesType(emb, typeAnnot, optEnv...) {
						return true
					}
				}
			}
			// Check if baseName is an interface
			var searchEnv *Environment
			if len(optEnv) > 0 && optEnv[0] != nil {
				searchEnv = optEnv[0]
			} else if si, ok := val.(*StructInstance); ok && si.Def != nil {
				searchEnv = si.Def.Env
			}
			if searchEnv != nil {
				var ifaceDef *InterfaceDefValue
				if strings.HasPrefix(baseName, "@") && strings.Contains(baseName, ".") {
					parts := strings.SplitN(baseName[1:], ".", 2)
					if modVal, found := searchEnv.GetModule(parts[0]); found {
						if expVal, ok := modVal.Exports[parts[1]]; ok {
							forcedExp := Force(expVal)
							if bf, isBF := forcedExp.(*BuiltinFunc); isBF && bf.TypeDef != nil {
								forcedExp = Force(bf.TypeDef)
							}
							if id, isIface := forcedExp.(*InterfaceDefValue); isIface {
								ifaceDef = id
							}
						}
					}
				} else if ifaceVal, found := searchEnv.Get(baseName); found {
					if id, isIface := Force(ifaceVal).(*InterfaceDefValue); isIface {
						ifaceDef = id
					}
				}
				if ifaceDef != nil {
					substIface := substituteInterfaceDef(ifaceDef, typeArgs)
					return valueImplementsInterfaceValue(val, substIface, searchEnv)
				}
			}
			return false
		}

		// Check struct type or embedded struct type
		if si, ok := val.(*StructInstance); ok {
			if si.Def.Name == typeAnnot {
				return true
			}
			for _, emb := range si.Embedded {
				if checkValueMatchesType(emb, typeAnnot, optEnv...) {
					return true
				}
			}
			// Check if typeAnnot is "Error"
			if typeAnnot == "Error" {
				if _, err := si.LookupMember("Message"); err == nil {
					return true
				}
				if _, err := si.LookupMember("Error"); err == nil {
					return true
				}
				return false
			}
		}

		var searchEnv *Environment
		if len(optEnv) > 0 && optEnv[0] != nil {
			searchEnv = optEnv[0]
		} else if si, ok := val.(*StructInstance); ok && si.Def != nil {
			searchEnv = si.Def.Env
		}

		if searchEnv != nil {
			// Check if typeAnnot is module qualified: @io.Reader or @bufio.Reader
			if strings.HasPrefix(typeAnnot, "@") && strings.Contains(typeAnnot, ".") {
				parts := strings.SplitN(typeAnnot[1:], ".", 2)
				modName := parts[0]
				typeName := parts[1]
				if modVal, found := searchEnv.GetModule(modName); found {
					if expVal, ok := modVal.Exports[typeName]; ok {
						forcedExp := Force(expVal)
						// If the exported item is a constructor BuiltinFunc with TypeDef, unwrap it
						if bf, isBF := forcedExp.(*BuiltinFunc); isBF && bf.TypeDef != nil {
							forcedExp = Force(bf.TypeDef)
						}
						if ifaceDef, isIface := forcedExp.(*InterfaceDefValue); isIface {
							return valueImplementsInterfaceValue(val, ifaceDef, searchEnv)
						}
						if structDef, isStruct := forcedExp.(*StructDefValue); isStruct {
							if si, ok := val.(*StructInstance); ok && si.Def != nil {
								if si.Def == structDef {
									return true
								}
								for _, emb := range si.Embedded {
									if emb.Def == structDef {
										return true
									}
								}
							}
						}
					}
				}
			}

			// Check if typeAnnot is an Interface in searchEnv
			if ifaceVal, found := searchEnv.Get(typeAnnot); found {
				if ifaceDef, isIface := Force(ifaceVal).(*InterfaceDefValue); isIface {
					return valueImplementsInterfaceValue(val, ifaceDef, searchEnv)
				}
			}
		}

		return false
	}
}

func checkThunkInferredType(thunk *Thunk, targetAnnot string, optEnv ...*Environment) bool {
	if thunk == nil || thunk.InferredType == nil {
		return true // unknown inferred type, preserve laziness
	}
	infT, ok := thunk.InferredType.(types.Type)
	if !ok || infT == nil || infT == types.TypeUnknown {
		return true // TypeUnknown could be anything at runtime, preserve laziness
	}

	targetAnnot = strings.TrimSpace(targetAnnot)
	if targetAnnot == "" || targetAnnot == "Unknown" {
		return true
	}

	var targetType types.Type
	var err error
	if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
		if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
			targetType, err = inf.ParseTypeAnnot(targetAnnot)
		}
	}
	if targetType == nil {
		targetType, err = types.ParseTypeAnnot(targetAnnot)
	}
	if err != nil {
		return true // target annot could not be parsed statically, preserve laziness
	}

	// If the statically inferred type is not assignable to targetType,
	// it can never match at runtime: return false without forcing the thunk!
	if !types.IsAssignableTo(infT, targetType) {
		return false
	}
	return true
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	pDepth := 0
	bDepth := 0
	brDepth := 0
	start := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '(' {
			pDepth++
		} else if r == ')' {
			pDepth--
		} else if r == '[' {
			bDepth++
		} else if r == ']' {
			bDepth--
		} else if r == '{' {
			brDepth++
		} else if r == '}' {
			brDepth--
		} else if r == ',' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
			parts = append(parts, strings.TrimSpace(string(runes[start:i])))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(string(runes[start:])))
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func valueImplementsInterfaceValue(val Value, iface *InterfaceDefValue, env *Environment) bool {
	hasUnboundTypeParam := func(annot string) bool {
		if len(iface.TypeArgs) > 0 || len(iface.TypeParams) == 0 {
			return false
		}
		runes := []rune(annot)
		for i := 0; i < len(runes); {
			if isIdentStart(runes[i]) {
				start := i
				for i < len(runes) && isIdentPart(runes[i]) {
					i++
				}
				word := string(runes[start:i])
				for _, tp := range iface.TypeParams {
					if tp.Name == word {
						return true
					}
				}
			} else {
				i++
			}
		}
		return false
	}

	// Check all direct methods required by the interface
	for mName, sig := range iface.Methods {
		expectedCount := 0
		for _, p := range sig.Params {
			if p.Name != "self" {
				expectedCount++
			}
		}
		if sig.BlockParam != nil {
			expectedCount++
		}
		var memberVal Value
		var err error
		if si, ok := val.(*StructInstance); ok {
			memberVal, err = si.LookupMember(mName)
		} else {
			memberVal, err = lookupBuiltinMember(val, mName)
		}
		if err != nil {
			return false
		}
		forced := Force(memberVal)
		switch fn := forced.(type) {
		case *Closure:
			// Check param count (excluding self)
			nonSelfCount := 0
			for _, p := range fn.Params {
				if p.Name != "self" {
					nonSelfCount++
				}
			}
			if fn.BlockParam != nil {
				nonSelfCount++
			}
			if nonSelfCount != expectedCount {
				return false
			}
			if ft, ok := fn.FuncType.(*types.FuncType); ok && ft != nil {
				if sig.ReturnType != "" && sig.ReturnType != "Unknown" && ft.Ret != nil && !hasUnboundTypeParam(sig.ReturnType) {
					var inf *types.Inferencer
					if env != nil && env.Inferencer != nil {
						if i, ok := env.Inferencer.(*types.Inferencer); ok {
							inf = i
						}
					}
					var expectedRetT types.Type
					if inf != nil {
						expectedRetT, _ = inf.ParseTypeAnnot(sig.ReturnType)
					}
					if expectedRetT == nil {
						expectedRetT, _ = types.ParseTypeAnnot(sig.ReturnType)
					}
					if expectedRetT != nil && !types.IsAssignableTo(ft.Ret, expectedRetT) {
						return false
					}
				}
				if env != nil && env.Inferencer != nil {
					if inf, ok := env.Inferencer.(*types.Inferencer); ok {
						if srcTup, ok := ft.Arg.(*types.TupleType); ok {
							paramIdx := 0
							for _, p := range sig.Params {
								if p.Name == "self" {
									continue
								}
								if p.TypeAnnot != "" && p.TypeAnnot != "Unknown" && !hasUnboundTypeParam(p.TypeAnnot) {
									if expectedParamT, err := inf.ParseTypeAnnot(p.TypeAnnot); err == nil {
										if paramIdx < len(srcTup.Elems) {
											if !types.IsAssignableTo(expectedParamT, srcTup.Elems[paramIdx]) {
												return false
											}
										}
									}
								}
								paramIdx++
							}
						}
					}
				}
			}
		case *BuiltinFunc:
			if si, ok := val.(*StructInstance); ok {
				if mName == "Type" {
					if expectedCount != 0 || (sig.ReturnType != "" && sig.ReturnType != "String") {
						return false
					}
				} else {
					if _, err := si.LookupMethod(mName, expectedCount); err != nil {
						return false
					}
				}
			} else {
				if ft, ok := fn.FuncType.(*types.FuncType); ok && ft != nil {
					minA := ft.MinArgs()
					maxA := ft.MaxArgs()
					if expectedCount < minA || (maxA >= 0 && expectedCount > maxA) {
						return false
					}
					if sig.ReturnType != "" && sig.ReturnType != "Unknown" && ft.Ret != nil && !hasUnboundTypeParam(sig.ReturnType) {
						if env != nil && env.Inferencer != nil {
							if inf, ok := env.Inferencer.(*types.Inferencer); ok {
								if expectedRetT, err := inf.ParseTypeAnnot(sig.ReturnType); err == nil {
									if !types.IsAssignableTo(ft.Ret, expectedRetT) {
										return false
									}
								}
							}
						}
					}
					if env != nil && env.Inferencer != nil {
						if inf, ok := env.Inferencer.(*types.Inferencer); ok {
							if srcTup, ok := ft.Arg.(*types.TupleType); ok {
								paramIdx := 0
								for _, p := range sig.Params {
									if p.Name == "self" {
										continue
									}
									if p.TypeAnnot != "" && p.TypeAnnot != "Unknown" && !hasUnboundTypeParam(p.TypeAnnot) {
										if expectedParamT, err := inf.ParseTypeAnnot(p.TypeAnnot); err == nil {
											if paramIdx < len(srcTup.Elems) {
												if !types.IsAssignableTo(expectedParamT, srcTup.Elems[paramIdx]) {
													return false
												}
											}
										}
									}
									paramIdx++
								}
							}
						}
					}
				}
				if mName == "Inspect" || mName == "Type" {
					if expectedCount != 0 || (sig.ReturnType != "" && sig.ReturnType != "String") {
						return false
					}
				} else if mName == "Len" || mName == "Ord" {
					if expectedCount != 0 || (sig.ReturnType != "" && sig.ReturnType != "Int") {
						return false
					}
				} else if mName == "Chr" {
					if expectedCount != 0 || (sig.ReturnType != "" && sig.ReturnType != "String") {
						return false
					}
				} else if iface.Name == "Equal" || iface.Name == "Comparable" {
					if expectedCount != 1 || (sig.ReturnType != "" && sig.ReturnType != "Bool") {
						return false
					}
				} else if iface.Name == "Indexable" {
					if mName == "Len" {
						if expectedCount != 0 || (sig.ReturnType != "" && sig.ReturnType != "Int") {
							return false
						}
					} else if mName == "[]" {
						if expectedCount != 1 {
							return false
						}
					}
				}
			}
		default:

			return false
		}
	}
	// Check embedded interfaces
	for _, embName := range iface.Embedded {
		var embDef *InterfaceDefValue
		embBase, embArgs, isGen := parseGenericTypeAnnot(embName)
		lookupName := embName
		if isGen {
			lookupName = embBase
		}
		if env != nil {
			// If iface belongs to a specific module, resolve embedded interface from that owning module first
			if iface.ModuleName != "" {
				if mod, found := env.GetModule(iface.ModuleName); found {
					if expVal, exists := mod.Exports[lookupName]; exists {
						if ed, isIface := Force(expVal).(*InterfaceDefValue); isIface {
							embDef = ed
						}
					}
				}
			}
			if embDef == nil {
				if embVal, found := env.Get(lookupName); found {
					if ed, isIface := Force(embVal).(*InterfaceDefValue); isIface {
						embDef = ed
					}
				}
			}
			if embDef == nil {
				// Try finding in imported modules across environment hierarchy
				for cur := env; cur != nil; cur = cur.parent {
					for _, mod := range cur.modules {
						if expVal, exists := mod.Exports[lookupName]; exists {
							if ed, isIface := Force(expVal).(*InterfaceDefValue); isIface {
								embDef = ed
								break
							}
						}
					}
					if embDef != nil {
						break
					}
				}
			}
		}
		if embDef == nil {
			return false
		}
		targetDef := embDef
		if isGen {
			targetDef = substituteInterfaceDef(embDef, embArgs)
		}
		if !valueImplementsInterfaceValue(val, targetDef, env) {
			return false
		}
	}
	return true
}

func structImplementsInterfaceValue(si *StructInstance, iface *InterfaceDefValue) bool {
	var env *Environment
	if si.Def != nil {
		env = si.Def.Env
	}
	return valueImplementsInterfaceValue(si, iface, env)
}
