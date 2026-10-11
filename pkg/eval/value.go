package eval

import (
	"fmt"
	"strings"

	"les/pkg/ast"
)

// Value represents evaluated runtime value in Les.
type Value interface {
	String() string
}

type IntValue int64

func (v IntValue) String() string { return fmt.Sprintf("%d", v) }

type FloatValue float64

func (v FloatValue) String() string { return fmt.Sprintf("%g", v) }

type StringValue string

func (v StringValue) String() string { return string(v) }

type BoolValue bool

func (v BoolValue) String() string {
	if v {
		return "True"
	}
	return "False"
}

type NilValue struct{}

func (v NilValue) String() string { return "Nil" }

// TupleValue holds lazy values (Thunks or direct values).
type TupleValue struct {
	Elements []Value
}

func (t *TupleValue) String() string {
	return t.stringWithDepth(0)
}

func (t *TupleValue) stringWithDepth(depth int) string {
	if depth > 10 {
		return "(...)"
	}
	var sb strings.Builder
	sb.WriteString("(")
	for i, el := range t.Elements {
		if i > 0 {
			sb.WriteString(", ")
		}
		val := Force(el)
		if innerT, ok := val.(*TupleValue); ok {
			sb.WriteString(innerT.stringWithDepth(depth + 1))
		} else {
			sb.WriteString(val.String())
		}
	}
	if len(t.Elements) == 1 {
		sb.WriteString(",")
	}
	sb.WriteString(")")
	return sb.String()
}

// DictValue holds key-value mapping.
type DictValue struct {
	Entries map[string]Value
}

func (d *DictValue) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	idx := 0
	for k, v := range d.Entries {
		if idx > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%q: %s", k, Force(v).String()))
		idx++
	}
	sb.WriteString("}")
	return sb.String()
}

// Closure represents user defined function with closure environment.
type Closure struct {
	Params          []ast.FuncParam
	Body            []ast.Stmt
	Env             *Environment
	IsFuncDef       bool
	EnclosingFuncID uint64
}

func (c *Closure) String() string { return "<function>" }

// BuiltinFunc represents built-in effect / primitive functions.
type BuiltinFunc struct {
	Name string
	Fn   func(args []Value) (Value, error)
}

func (b *BuiltinFunc) String() string { return fmt.Sprintf("<builtin %s>", b.Name) }

// ModuleValue represents an imported or registered module namespace.
type ModuleValue struct {
	Name    string
	Exports map[string]Value
}

func (m *ModuleValue) String() string { return fmt.Sprintf("<module %s>", m.Name) }

// StructDefValue represents struct type definition (usable as constructor).
type StructDefValue struct {
	Name          string
	FieldOrder    []string
	FieldDefaults map[string]ast.Expr
	EmbeddedNames []string
	Methods       map[string]*ast.FuncDef
	Env           *Environment
}

func (s *StructDefValue) String() string {
	return fmt.Sprintf("<struct %s>", s.Name)
}

// StructInstance represents an instantiated struct value.
type StructInstance struct {
	Def      *StructDefValue
	Fields   map[string]Value
	Embedded map[string]*StructInstance
}

func (si *StructInstance) String() string {
	var sb strings.Builder
	sb.WriteString(si.Def.Name)
	sb.WriteString("(")
	for i, fName := range si.Def.FieldOrder {
		if i > 0 {
			sb.WriteString(", ")
		}
		val := si.Fields[fName]
		if val != nil {
			sb.WriteString(fmt.Sprintf("%s: %s", fName, Force(val).String()))
		} else {
			sb.WriteString(fmt.Sprintf("%s: ()", fName))
		}
	}
	sb.WriteString(")")
	return sb.String()
}

func (si *StructInstance) bindMethod(m *ast.FuncDef) *Closure {
	methodEnv := NewEnvironment(si.Def.Env)
	methodEnv.Set("self", si)

	// Populate promoted fields from embedded structs
	var populateEmbeddedFields func(target *StructInstance)
	populateEmbeddedFields = func(target *StructInstance) {
		for _, emb := range target.Embedded {
			populateEmbeddedFields(emb)
			for fName, fVal := range emb.Fields {
				methodEnv.Set(fName, fVal)
			}
		}
	}
	populateEmbeddedFields(si)

	// Populate own fields (overriding embedded fields)
	for fName, fVal := range si.Fields {
		methodEnv.Set(fName, fVal)
	}

	params := m.Params
	if len(params) > 0 && params[0].Name == "self" {
		params = params[1:]
	}

	return &Closure{
		Params:          params,
		Body:            m.Body,
		Env:             methodEnv,
		IsFuncDef:       true,
		EnclosingFuncID: methodEnv.EnclosingFuncID,
	}
}

// LookupMember searches for field or method on StructInstance with Go-style promotion rules.
func (si *StructInstance) LookupMember(name string) (Value, error) {
	// 1. Direct fields
	if v, ok := si.Fields[name]; ok {
		return v, nil
	}

	// 2. Direct methods
	if m, ok := si.Def.Methods[name]; ok {
		return si.bindMethod(m), nil
	}

	// 3. BFS over embedded structs
	currentLevel := make([]*StructInstance, 0, len(si.Embedded))
	for _, embName := range si.Def.EmbeddedNames {
		if emb, ok := si.Embedded[embName]; ok {
			currentLevel = append(currentLevel, emb)
		}
	}

	visited := make(map[string]bool)
	visited[si.Def.Name] = true

	for len(currentLevel) > 0 {
		type candidate struct {
			val    Value
			origin string
		}
		var candidates []candidate
		var nextLevel []*StructInstance

		for _, emb := range currentLevel {
			if visited[emb.Def.Name] {
				continue
			}
			visited[emb.Def.Name] = true

			if v, ok := emb.Fields[name]; ok {
				candidates = append(candidates, candidate{val: v, origin: emb.Def.Name})
			}
			if m, ok := emb.Def.Methods[name]; ok {
				candidates = append(candidates, candidate{val: emb.bindMethod(m), origin: emb.Def.Name})
			}

			for _, subName := range emb.Def.EmbeddedNames {
				if sub, ok := emb.Embedded[subName]; ok {
					nextLevel = append(nextLevel, sub)
				}
			}
		}

		if len(candidates) == 1 {
			return candidates[0].val, nil
		}
		if len(candidates) > 1 {
			origins := make([]string, len(candidates))
			for i, c := range candidates {
				origins[i] = c.origin
			}
			return nil, fmt.Errorf("ambiguous selector '%s' on type %s (candidates in %s)", name, si.Def.Name, strings.Join(origins, ", "))
		}

		currentLevel = nextLevel
	}

	return nil, fmt.Errorf("type %s has no field or method '%s'", si.Def.Name, name)
}


// Thunk implements aggressive lazy evaluation (memoized call-by-need).
type Thunk struct {
	Evaluated bool
	Value     Value
	Expr      ast.Expr
	Env       *Environment
	Evaluating bool // for detecting recursive blackholes/cycles
}

func NewThunk(expr ast.Expr, env *Environment) *Thunk {
	return &Thunk{
		Evaluated: false,
		Expr:      expr,
		Env:       env,
	}
}

func (t *Thunk) String() string {
	if t.Evaluated {
		return t.Value.String()
	}
	return "<thunk>"
}

// Force drives evaluation of Thunk until a concrete non-Thunk value is reached.
func Force(v Value) Value {
	for {
		thunk, ok := v.(*Thunk)
		if !ok {
			return v
		}
		if thunk.Evaluated {
			v = thunk.Value
			continue
		}
		if thunk.Evaluating {
			panic("blackhole: circular reference detected in lazy thunk")
		}
		thunk.Evaluating = true
		val, err := thunk.Env.evalExpr(thunk.Expr)
		if err != nil {
			panic(fmt.Sprintf("runtime error in thunk: %v", err))
		}
		thunk.Value = val // do not force lazily wrapped container elements
		thunk.Evaluated = true
		thunk.Evaluating = false
		v = thunk.Value
	}
}

// Environment stores bindings in current scope.
type Environment struct {
	store           map[string]Value
	modules         map[string]*ModuleValue
	parent          *Environment
	Output          *strings.Builder                        // captures print output if specified
	Loader          func(path string) (*ModuleValue, error) // module loader callback
	EnclosingFuncID uint64                                  // 0 means top-level
	ActiveClosureID uint64                                  // 0 means not directly inside a closure call
}

func NewEnvironment(parent *Environment) *Environment {
	env := &Environment{
		store:   make(map[string]Value),
		modules: make(map[string]*ModuleValue),
		parent:  parent,
	}
	if parent != nil {
		env.Output = parent.Output
		env.Loader = parent.Loader
		env.EnclosingFuncID = parent.EnclosingFuncID
		env.ActiveClosureID = parent.ActiveClosureID
	}
	return env
}

// ReturnSignal carries the returned value and the target function call ID.
type ReturnSignal struct {
	FuncID uint64
	Value  Value
}

// BreakSignal carries the break value and the target closure call ID.
type BreakSignal struct {
	ClosureID uint64
	Value     Value
}

func (env *Environment) Get(name string) (Value, bool) {
	if v, ok := env.store[name]; ok {
		return v, true
	}
	if env.parent != nil {
		return env.parent.Get(name)
	}
	return nil, false
}

func (env *Environment) Set(name string, val Value) {
	env.store[name] = val
}

func (env *Environment) RegisterModule(mod *ModuleValue) {
	env.modules[mod.Name] = mod
}

func (env *Environment) GetModule(name string) (*ModuleValue, bool) {
	if m, ok := env.modules[name]; ok {
		return m, true
	}
	if env.parent != nil {
		return env.parent.GetModule(name)
	}
	return nil, false
}
