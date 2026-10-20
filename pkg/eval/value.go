package eval

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"

	"les/pkg/ast"
	"les/pkg/types"
)

// Value represents evaluated runtime value in Les.
type Value interface {
	String() string
}

// IntValue represents an arbitrary-precision integer in Les.
// If the value fits in [math.MinInt64, math.MaxInt64], it is stored in small and big is nil.
// If the value overflows int64, big stores the *big.Int and small is 0.
type IntValue struct {
	small int64
	big   *big.Int
}

// NewIntValue creates an IntValue from an int64.
func NewIntValue(n int64) IntValue {
	return IntValue{small: n}
}

// NewIntValueFromBig creates an IntValue from a *big.Int, automatically demoting to int64 if it fits.
func NewIntValueFromBig(z *big.Int) IntValue {
	if z == nil {
		return IntValue{}
	}
	if z.IsInt64() {
		return IntValue{small: z.Int64()}
	}
	return IntValue{big: new(big.Int).Set(z)}
}

// IsBig reports whether the integer exceeds the int64 range.
func (v IntValue) IsBig() bool {
	return v.big != nil
}

// Int64 returns the int64 value if it fits, or (0, false) if it is a multi-precision integer.
func (v IntValue) Int64() (int64, bool) {
	if v.big != nil {
		return 0, false
	}
	return v.small, true
}

// ToInt returns the Go int value if it fits in int, or (0, false) otherwise.
func (v IntValue) ToInt() (int, bool) {
	if v.big != nil {
		return 0, false
	}
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1
	if v.small < int64(minInt) || v.small > int64(maxInt) {
		return 0, false
	}
	return int(v.small), true
}

// ToBig returns a *big.Int representing the value.
func (v IntValue) ToBig() *big.Int {
	if v.big != nil {
		return new(big.Int).Set(v.big)
	}
	return big.NewInt(v.small)
}

func (v IntValue) String() string {
	if v.big != nil {
		return v.big.String()
	}
	return strconv.FormatInt(v.small, 10)
}

// Format formats the integer in the specified radix (2 to 36).
func (v IntValue) Format(radix int) (string, error) {
	if radix < 2 || radix > 36 {
		return "", fmt.Errorf("unsupported radix %d", radix)
	}
	if v.big != nil {
		return v.big.Text(radix), nil
	}
	return strconv.FormatInt(v.small, radix), nil
}

// Equal reports whether two IntValue instances have equal value.
func (v IntValue) Equal(other IntValue) bool {
	if v.big == nil && other.big == nil {
		return v.small == other.small
	}
	if (v.big == nil) != (other.big == nil) {
		return false
	}
	return v.big.Cmp(other.big) == 0
}

// Cmp compares v and other and returns:
// -1 if v <  other
//  0 if v == other
// +1 if v >  other
func (v IntValue) Cmp(other IntValue) int {
	if v.big == nil && other.big == nil {
		if v.small < other.small {
			return -1
		} else if v.small > other.small {
			return 1
		}
		return 0
	}
	return v.ToBig().Cmp(other.ToBig())
}

// Neg negates the integer value, promoting to big.Int if -math.MinInt64.
func (v IntValue) Neg() IntValue {
	if v.big == nil {
		if v.small == math.MinInt64 {
			return NewIntValueFromBig(new(big.Int).Neg(big.NewInt(math.MinInt64)))
		}
		return NewIntValue(-v.small)
	}
	return NewIntValueFromBig(new(big.Int).Neg(v.big))
}

// Add returns v + other.
func (v IntValue) Add(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		res := v.small + other.small
		if (res^v.small)&(res^other.small) >= 0 {
			return NewIntValue(res)
		}
	}
	return NewIntValueFromBig(new(big.Int).Add(v.ToBig(), other.ToBig()))
}

// Sub returns v - other.
func (v IntValue) Sub(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		res := v.small - other.small
		if (res^v.small)&^(res^other.small) >= 0 {
			return NewIntValue(res)
		}
	}
	return NewIntValueFromBig(new(big.Int).Sub(v.ToBig(), other.ToBig()))
}

// Mul returns v * other.
func (v IntValue) Mul(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		if v.small == 0 || other.small == 0 {
			return NewIntValue(0)
		}
		res := v.small * other.small
		if res/other.small == v.small && !(v.small == -1 && other.small == math.MinInt64) && !(other.small == -1 && v.small == math.MinInt64) {
			return NewIntValue(res)
		}
	}
	return NewIntValueFromBig(new(big.Int).Mul(v.ToBig(), other.ToBig()))
}

// Quo returns v / other (integer truncated division).
func (v IntValue) Quo(other IntValue) (IntValue, error) {
	if other.big == nil && other.small == 0 {
		return IntValue{}, fmt.Errorf("division by zero")
	}
	if v.big == nil && other.big == nil {
		if v.small == math.MinInt64 && other.small == -1 {
			return NewIntValueFromBig(new(big.Int).Quo(big.NewInt(math.MinInt64), big.NewInt(-1))), nil
		}
		return NewIntValue(v.small / other.small), nil
	}
	return NewIntValueFromBig(new(big.Int).Quo(v.ToBig(), other.ToBig())), nil
}

// Rem returns v % other (integer remainder).
func (v IntValue) Rem(other IntValue) (IntValue, error) {
	if other.big == nil && other.small == 0 {
		return IntValue{}, fmt.Errorf("division by zero")
	}
	if v.big == nil && other.big == nil {
		if v.small == math.MinInt64 && other.small == -1 {
			return NewIntValue(0), nil
		}
		return NewIntValue(v.small % other.small), nil
	}
	return NewIntValueFromBig(new(big.Int).Rem(v.ToBig(), other.ToBig())), nil
}

// Not returns ^v (bitwise NOT / two's complement complement).
func (v IntValue) Not() IntValue {
	if v.big == nil {
		return NewIntValue(^v.small)
	}
	return NewIntValueFromBig(new(big.Int).Not(v.big))
}

// And returns v & other.
func (v IntValue) And(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		return NewIntValue(v.small & other.small)
	}
	return NewIntValueFromBig(new(big.Int).And(v.ToBig(), other.ToBig()))
}

// Or returns v | other.
func (v IntValue) Or(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		return NewIntValue(v.small | other.small)
	}
	return NewIntValueFromBig(new(big.Int).Or(v.ToBig(), other.ToBig()))
}

// Xor returns v ^ other.
func (v IntValue) Xor(other IntValue) IntValue {
	if v.big == nil && other.big == nil {
		return NewIntValue(v.small ^ other.small)
	}
	return NewIntValueFromBig(new(big.Int).Xor(v.ToBig(), other.ToBig()))
}

// Lsh returns v << shift with Ruby semantics (negative shift acts as Rsh).
func (v IntValue) Lsh(shift IntValue) (IntValue, error) {
	if shift.Cmp(NewIntValue(0)) < 0 {
		return v.Rsh(shift.Neg())
	}
	if (v.big == nil && v.small == 0) || (shift.big == nil && shift.small == 0) {
		return v, nil
	}
	if shift.big != nil || shift.small > 1<<26 {
		return IntValue{}, fmt.Errorf("shift count too large: %s", shift.String())
	}
	z := new(big.Int).Lsh(v.ToBig(), uint(shift.small))
	return NewIntValueFromBig(z), nil
}

// Rsh returns v >> shift with Ruby semantics (negative shift acts as Lsh).
func (v IntValue) Rsh(shift IntValue) (IntValue, error) {
	if shift.Cmp(NewIntValue(0)) < 0 {
		return v.Lsh(shift.Neg())
	}
	if shift.big == nil && shift.small == 0 {
		return v, nil
	}
	if v.big == nil && v.small == 0 {
		return v, nil
	}
	if shift.big != nil || shift.small > 1<<26 {
		if v.Cmp(NewIntValue(0)) < 0 {
			return NewIntValue(-1), nil
		}
		return NewIntValue(0), nil
	}
	z := new(big.Int).Rsh(v.ToBig(), uint(shift.small))
	return NewIntValueFromBig(z), nil
}

type FloatValue float64

func (v FloatValue) String() string { return fmt.Sprintf("%g", v) }

type StringValue string

func (v StringValue) String() string { return string(v) }

type BytesValue string

func (v BytesValue) String() string { return hex.EncodeToString([]byte(v)) }

type BoolValue bool

func (v BoolValue) String() string {
	if v {
		return "true"
	}
	return "false"
}

type NilValue struct{}

func (v NilValue) String() string { return "nil" }

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

// DictKey uniquely identifies a dictionary key while strictly preserving its type and value.
type DictKey struct {
	Type string // Type tag: "Int", "Float", "String", "Bytes", "Bool", "Nil", "Tuple", etc.
	Key  string // Representation preserving the value within that type
}

// MakeDictKey converts a runtime Value into a typed DictKey.
func MakeDictKey(v Value) DictKey {
	v = Force(v)
	switch val := v.(type) {
	case IntValue:
		return DictKey{Type: "Int", Key: val.String()}
	case FloatValue:
		return DictKey{Type: "Float", Key: fmt.Sprintf("%g", float64(val))}
	case StringValue:
		return DictKey{Type: "String", Key: string(val)}
	case BytesValue:
		return DictKey{Type: "Bytes", Key: string(val)}
	case BoolValue:
		if val {
			return DictKey{Type: "Bool", Key: "true"}
		}
		return DictKey{Type: "Bool", Key: "false"}
	case NilValue:
		return DictKey{Type: "Nil", Key: "nil"}
	case *TupleValue:
		var sb strings.Builder
		sb.WriteString("(")
		for i, el := range val.Elements {
			if i > 0 {
				sb.WriteString(",")
			}
			elemKey := MakeDictKey(el)
			sb.WriteString(elemKey.Type)
			sb.WriteString(":")
			sb.WriteString(elemKey.Key)
		}
		sb.WriteString(")")
		return DictKey{Type: "Tuple", Key: sb.String()}
	default:
		return DictKey{Type: fmt.Sprintf("%T", val), Key: val.String()}
	}
}

// DictValue holds key-value mapping with strictly typed keys.
type DictValue struct {
	Entries   map[DictKey]Value
	KeyValues map[DictKey]Value
}

// NewDictValue creates a DictValue ensuring Entries and KeyValues are non-nil.
func NewDictValue(entries map[DictKey]Value, keyValues ...map[DictKey]Value) *DictValue {
	if entries == nil {
		entries = make(map[DictKey]Value)
	}
	kv := make(map[DictKey]Value)
	if len(keyValues) > 0 && keyValues[0] != nil {
		kv = keyValues[0]
	} else {
		for dk := range entries {
			kv[dk] = StringValue(dk.Key)
		}
	}
	return &DictValue{
		Entries:   entries,
		KeyValues: kv,
	}
}

func (d *DictValue) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	idx := 0
	for dk, v := range d.Entries {
		if idx > 0 {
			sb.WriteString(", ")
		}
		var kStr string
		if origKey, ok := d.KeyValues[dk]; ok && origKey != nil {
			if _, isStr := origKey.(StringValue); isStr {
				kStr = fmt.Sprintf("%q", origKey.String())
			} else {
				kStr = origKey.String()
			}
		} else {
			if dk.Type == "String" {
				kStr = fmt.Sprintf("%q", dk.Key)
			} else {
				kStr = dk.Key
			}
		}
		sb.WriteString(fmt.Sprintf("%s: %s", kStr, Force(v).String()))
		idx++
	}
	sb.WriteString("}")
	return sb.String()
}

// Closure represents user defined function with closure environment.
type Closure struct {
	Params          []ast.FuncParam
	BlockParam      *ast.FuncParam
	Body            []ast.Stmt
	Env             *Environment
	IsFuncDef       bool
	EnclosingFuncID uint64
	FuncType        any // types.Type if known, for type reflection
}

func (c *Closure) String() string { return "<function>" }

// BuiltinFunc represents built-in effect / primitive functions.
type BuiltinFunc struct {
	Name     string
	Fn       func(args []Value) (Value, error)
	TypeDef  Value // optional underlying StructDefValue or InterfaceDefValue (for type checking / constructors)
	FuncType any   // types.Type if known, for type reflection
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
	AllMethods    map[string][]*ast.FuncDef
	Env           *Environment
	StructType    any // Inferred *types.StructType
}

func (s *StructDefValue) String() string {
	return fmt.Sprintf("<struct %s>", s.Name)
}

// InterfaceDefValue represents interface type definition.
type InterfaceDefValue struct {
	Name       string
	ModuleName string
	Embedded   []string
	Methods    map[string]ast.MethodSig
}

func (i *InterfaceDefValue) String() string {
	return fmt.Sprintf("<interface %s>", i.Name)
}

// StructInstance represents an instantiated struct value.
type StructInstance struct {
	Def            *StructDefValue
	Fields         map[string]Value
	Embedded       map[string]*StructInstance
	IsInitializing bool
}

func (si *StructInstance) String() string {
	if m, err := si.LookupMember("ToString"); err == nil {
		if fn, isClosure := Force(m).(*Closure); isClosure {
			if res, err := fn.Env.invokeClosure(fn, nil); err == nil {
				return Force(res).String()
			}
		}
	}

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

	closure := &Closure{
		Params:          params,
		BlockParam:      m.BlockParam,
		Body:            m.Body,
		Env:             methodEnv,
		IsFuncDef:       true,
		EnclosingFuncID: methodEnv.EnclosingFuncID,
	}
	if si.Def != nil && si.Def.StructType != nil {
		if st, ok := si.Def.StructType.(*types.StructType); ok {
			if mType, ok := st.Methods[m.Name]; ok {
				closure.FuncType = mType
			}
		}
	}

	return closure
}

func nonSelfParamCount(m *ast.FuncDef) int {
	count := 0
	for _, p := range m.Params {
		if p.Name != "self" {
			count++
		}
	}
	if m.BlockParam != nil {
		count++
	}
	return count
}

func (si *StructInstance) makeDispatcher(name string, methods []*ast.FuncDef) Value {
	if len(methods) == 1 {
		return si.bindMethod(methods[0])
	}
	bf := &BuiltinFunc{
		Name: name,
		Fn: func(args []Value) (Value, error) {
			for _, m := range methods {
				if nonSelfParamCount(m) == len(args) {
					c := si.bindMethod(m)
					return c.Env.invokeClosure(c, args)
				}
			}
			return nil, fmt.Errorf("method %s on %s called with %d arguments", name, si.Def.Name, len(args))
		},
	}
	if si.Def != nil && si.Def.StructType != nil {
		if st, ok := si.Def.StructType.(*types.StructType); ok {
			if mType, ok := st.Methods[name]; ok {
				bf.FuncType = mType
			}
		}
	}
	return bf
}

// LookupMethod searches for method on StructInstance matching arity.
func (si *StructInstance) LookupMethod(name string, arity int) (*Closure, error) {
	if si.Def != nil && si.Def.AllMethods != nil {
		for _, m := range si.Def.AllMethods[name] {
			if nonSelfParamCount(m) == arity {
				return si.bindMethod(m), nil
			}
		}
	} else if si.Def != nil && si.Def.Methods != nil {
		if m, ok := si.Def.Methods[name]; ok && nonSelfParamCount(m) == arity {
			return si.bindMethod(m), nil
		}
	}

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
			closure *Closure
			origin  string
		}
		var candidates []candidate
		var nextLevel []*StructInstance

		for _, emb := range currentLevel {
			if visited[emb.Def.Name] {
				continue
			}
			visited[emb.Def.Name] = true

			if emb.Def != nil && emb.Def.AllMethods != nil {
				for _, m := range emb.Def.AllMethods[name] {
					if nonSelfParamCount(m) == arity {
						candidates = append(candidates, candidate{closure: emb.bindMethod(m), origin: emb.Def.Name})
					}
				}
			} else if emb.Def != nil && emb.Def.Methods != nil {
				if m, ok := emb.Def.Methods[name]; ok && nonSelfParamCount(m) == arity {
					candidates = append(candidates, candidate{closure: emb.bindMethod(m), origin: emb.Def.Name})
				}
			}

			for _, subName := range emb.Def.EmbeddedNames {
				if sub, ok := emb.Embedded[subName]; ok {
					nextLevel = append(nextLevel, sub)
				}
			}
		}

		if len(candidates) == 1 {
			return candidates[0].closure, nil
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

	return nil, fmt.Errorf("type %s has no method '%s' with arity %d", si.Def.Name, name, arity)
}

// LookupMember searches for field or method on StructInstance with Go-style promotion rules.
func (si *StructInstance) LookupMember(name string) (Value, error) {
	// 1. Direct fields
	if v, ok := si.Fields[name]; ok {
		return v, nil
	}

	// 2. Direct methods
	if si.Def != nil {
		if ms, ok := si.Def.AllMethods[name]; ok && len(ms) > 0 {
			return si.makeDispatcher(name, ms), nil
		}
		if m, ok := si.Def.Methods[name]; ok {
			return si.bindMethod(m), nil
		}
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
			if emb.Def != nil && emb.Def.AllMethods != nil {
				if ms, ok := emb.Def.AllMethods[name]; ok && len(ms) > 0 {
					candidates = append(candidates, candidate{val: emb.makeDispatcher(name, ms), origin: emb.Def.Name})
				}
			} else if emb.Def != nil && emb.Def.Methods != nil {
				if m, ok := emb.Def.Methods[name]; ok {
					candidates = append(candidates, candidate{val: emb.bindMethod(m), origin: emb.Def.Name})
				}
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

	if name == "Type" {
		return &BuiltinFunc{
			Name: "Type",
			Fn: func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("Type() takes 0 arguments, got %d", len(args))
				}
				return StringValue(si.Def.Name), nil
			},
		}, nil
	}

	return nil, fmt.Errorf("type %s has no field or method '%s'", si.Def.Name, name)
}

// Thunk implements aggressive lazy evaluation (memoized call-by-need).
type Thunk struct {
	Evaluated    bool
	Value        Value
	Expr         ast.Expr
	Env          *Environment
	Producer     func() (Value, error) // for dynamic/I/O thunks not bound to AST Expr
	Evaluating   bool                  // for detecting recursive blackholes/cycles
	InferredType any                   // types.Type if known, for non-forcing type checks
}

func NewThunk(expr ast.Expr, env *Environment) *Thunk {
	if val, ok := evalLiteral(expr); ok {
		return &Thunk{
			Evaluated: true,
			Value:     val,
			Expr:      expr,
			Env:       env,
		}
	}
	return &Thunk{
		Evaluated: false,
		Expr:      expr,
		Env:       env,
	}
}

// NewActionThunk creates a lazy Thunk whose value is produced by an action when demanded.
func NewActionThunk(producer func() (Value, error)) *Thunk {
	return &Thunk{
		Evaluated: false,
		Producer:  producer,
	}
}

func (t *Thunk) String() string {
	if t.Evaluated {
		return t.Value.String()
	}
	return "<thunk>"
}

// PeekEvaluated returns the evaluated value if v is a non-Thunk or an already-evaluated Thunk.
// If v is an unevaluated Thunk, it returns (nil, false).
func PeekEvaluated(v Value) (Value, bool) {
	for {
		thunk, ok := v.(*Thunk)
		if !ok {
			return v, true
		}
		if !thunk.Evaluated {
			return nil, false
		}
		v = thunk.Value
	}
}

// FormatRuntimeError attaches file and position information to a runtime error if not already formatted.
func FormatRuntimeError(filename string, pos ast.Position, err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, " at col ") {
		return err
	}
	if pos.Line > 0 {
		if filename != "" {
			return fmt.Errorf("%s:%d: %v at col %d", filename, pos.Line, err, pos.Col)
		}
		return fmt.Errorf("%d: %v at col %d", pos.Line, err, pos.Col)
	}
	if filename != "" {
		return fmt.Errorf("%s: %v", filename, err)
	}
	return err
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
		var val Value
		var err error
		if thunk.Producer != nil {
			val, err = thunk.Producer()
		} else if thunk.Expr != nil && thunk.Env != nil {
			val, err = thunk.Env.evalExpr(thunk.Expr)
		} else {
			val = &TupleValue{Elements: []Value{}}
		}
		if err != nil {
			pos := ast.Position{}
			if thunk.Expr != nil {
				pos = thunk.Expr.Pos()
			}
			fn := ""
			if thunk.Env != nil {
				fn = thunk.Env.Filename
			}
			formatted := FormatRuntimeError(fn, pos, err)
			panic(fmt.Sprintf("runtime error in thunk: %v", formatted))
		}
		thunk.Value = val // do not force lazily wrapped container elements
		thunk.Evaluated = true
		thunk.Evaluating = false
		v = thunk.Value
	}
}

// Environment stores bindings in current scope.
type Environment struct {
	store              map[string]Value
	modules            map[string]*ModuleValue
	parent             *Environment
	Inferencer         any                                     // *types.Inferencer if available
	RawStdin           io.Reader                               // raw standard input
	RawStdout          io.Writer                               // raw standard output
	RawStderr          io.Writer                               // raw standard error
	StdinReader        *bufio.Reader                           // shared buffered reader for input
	StdoutWriter       *bufio.Writer                           // shared buffered writer for output
	ActiveBufioWriters []*bufio.Writer                         // all active bufio.Writers created in this environment tree
	bufioUnderlying    map[*bufio.Writer]io.Writer             // underlying target writer for custom bufio writers
	Loader             func(path string) (*ModuleValue, error) // module loader callback
	EnclosingFuncID    uint64                                  // 0 means top-level
	ActiveClosureID    uint64                                  // 0 means not directly inside a closure call
	ActiveLoopID       uint64                                  // 0 means not inside a loop
	Filename           string                                  // source file name for error reporting
}

func NewEnvironment(parent *Environment) *Environment {
	env := &Environment{
		store:           make(map[string]Value),
		modules:         make(map[string]*ModuleValue),
		parent:          parent,
		bufioUnderlying: make(map[*bufio.Writer]io.Writer),
	}
	if parent != nil {
		env.Loader = parent.Loader
		env.Inferencer = parent.Inferencer
		env.EnclosingFuncID = parent.EnclosingFuncID
		env.ActiveClosureID = parent.ActiveClosureID
		env.ActiveLoopID = parent.ActiveLoopID
		env.Filename = parent.Filename
	}
	return env
}

// ReturnSignal carries the returned value and the target function call ID.
type ReturnSignal struct {
	FuncID   uint64
	Value    Value
	Pos      ast.Position
	Filename string
}

// BreakSignal carries the break value and the target closure/loop call ID.
type BreakSignal struct {
	ClosureID uint64
	LoopID    uint64
	Value     Value
	Pos       ast.Position
	Filename  string
}

// ContinueSignal signals skipping to the next iteration of the loop.
type ContinueSignal struct {
	ClosureID uint64
	LoopID    uint64
	Pos       ast.Position
	Filename  string
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

func (env *Environment) Update(name string, val Value) bool {
	if _, ok := env.store[name]; ok {
		env.store[name] = val
		return true
	}
	if env.parent != nil {
		return env.parent.Update(name, val)
	}
	return false
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

// GetRawStdin returns the raw stdin reader, falling back to os.Stdin.
func (env *Environment) GetRawStdin() io.Reader {
	if env.parent != nil {
		return env.parent.GetRawStdin()
	}
	if env.RawStdin == nil {
		return os.Stdin
	}
	return env.RawStdin
}

// GetRawStdout returns the raw stdout writer, falling back to os.Stdout.
func (env *Environment) GetRawStdout() io.Writer {
	if env.parent != nil {
		return env.parent.GetRawStdout()
	}
	if env.RawStdout == nil {
		return os.Stdout
	}
	return env.RawStdout
}

// GetRawStderr returns the raw stderr writer, falling back to os.Stderr.
func (env *Environment) GetRawStderr() io.Writer {
	if env.parent != nil {
		return env.parent.GetRawStderr()
	}
	if env.RawStderr == nil {
		return os.Stderr
	}
	return env.RawStderr
}

// GetStdinReader returns the shared buffered stdin reader, initializing with RawStdin if nil.
func (env *Environment) GetStdinReader() *bufio.Reader {
	if env.parent != nil {
		return env.parent.GetStdinReader()
	}
	if env.StdinReader == nil {
		env.StdinReader = bufio.NewReader(env.GetRawStdin())
	}
	return env.StdinReader
}

// GetStdoutWriter returns the shared buffered stdout writer, initializing with RawStdout if nil.
func (env *Environment) GetStdoutWriter() *bufio.Writer {
	if env.parent != nil {
		return env.parent.GetStdoutWriter()
	}
	if env.StdoutWriter == nil {
		env.StdoutWriter = bufio.NewWriter(env.GetRawStdout())
		env.RegisterBufioWriter(env.StdoutWriter, env.GetRawStdout())
	}
	return env.StdoutWriter
}

// RegisterBufioWriter tracks a bufio.Writer to be flushed at program termination.
func (env *Environment) RegisterBufioWriter(bw *bufio.Writer, underlying io.Writer) {
	if env.parent != nil {
		env.parent.RegisterBufioWriter(bw, underlying)
		return
	}
	if bw == nil {
		return
	}
	if env.bufioUnderlying == nil {
		env.bufioUnderlying = make(map[*bufio.Writer]io.Writer)
	}
	if underlying != nil {
		env.bufioUnderlying[bw] = underlying
	}
	for _, existing := range env.ActiveBufioWriters {
		if existing == bw {
			return
		}
	}
	env.ActiveBufioWriters = append(env.ActiveBufioWriters, bw)
}

// UnregisterBufioWriter removes a bufio.Writer from active flush tracking and underlying map.
func (env *Environment) UnregisterBufioWriter(bw *bufio.Writer) {
	if env.parent != nil {
		env.parent.UnregisterBufioWriter(bw)
		return
	}
	if bw == nil {
		return
	}
	var retained []*bufio.Writer
	for _, existing := range env.ActiveBufioWriters {
		if existing != bw {
			retained = append(retained, existing)
		}
	}
	env.ActiveBufioWriters = retained
	if env.bufioUnderlying != nil {
		delete(env.bufioUnderlying, bw)
	}
}

// GetActiveBufioWriters returns the active bufio writers tracked in the root environment.
func (env *Environment) GetActiveBufioWriters() []*bufio.Writer {
	if env.parent != nil {
		return env.parent.GetActiveBufioWriters()
	}
	return env.ActiveBufioWriters
}

// FlushAllBufioWriters flushes all tracked bufio.Writers iteratively until all buffered
// data is completely drained, properly handling writers whose Write method refills other writers
// (e.g. custom writer writing to Stdout) or dynamically registers new writers.
// Writers that fail or panic during flush are recorded and skipped on subsequent rounds.
func (env *Environment) FlushAllBufioWriters() error {
	if env.parent != nil {
		return env.parent.FlushAllBufioWriters()
	}
	var firstErr error
	failed := make(map[*bufio.Writer]bool)
	const maxRounds = 100 // protect against infinite refill cycles

	for round := 0; round < maxRounds; round++ {
		progress := false
		// Read slice index by index since env.ActiveBufioWriters might grow dynamically during callbacks
		for i := 0; i < len(env.ActiveBufioWriters); i++ {
			bw := env.ActiveBufioWriters[i]
			if failed[bw] {
				continue
			}
			beforeBuf := bw.Buffered()
			var roundErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						if exitErr, ok := r.(*ExitError); ok {
							roundErr = exitErr
						} else {
							roundErr = fmt.Errorf("panic during flush: %v", r)
						}
						failed[bw] = true
						if firstErr == nil {
							firstErr = roundErr
						}
					}
				}()
				if err := bw.Flush(); err != nil {
					roundErr = err
					failed[bw] = true
					if firstErr == nil {
						firstErr = err
					}
				}
			}()
			if roundErr == nil && beforeBuf > 0 && bw.Buffered() < beforeBuf {
				progress = true
			}
		}
		// Check if any non-failed writer still has buffered data or if new writers were added
		anyRemaining := false
		for _, bw := range env.ActiveBufioWriters {
			if !failed[bw] && bw.Buffered() > 0 {
				anyRemaining = true
				break
			}
		}
		if !anyRemaining {
			break
		}
		if !progress {
			// No bytes were successfully flushed in this round by remaining writers; break to avoid infinite loop
			break
		}
	}
	// If non-failed writers still have buffered data (e.g. infinite refill cycle or stuck progress), report error
	if firstErr == nil {
		for _, bw := range env.ActiveBufioWriters {
			if !failed[bw] && bw.Buffered() > 0 {
				firstErr = fmt.Errorf("buffered output remained unflushed after drain limit")
				break
			}
		}
	}
	// Clean up fully drained custom writers so they do not leak memory or outlive the evaluation.
	// Only retain StdoutWriter or writers that still have buffered data.
	var retained []*bufio.Writer
	for _, bw := range env.ActiveBufioWriters {
		if bw == env.StdoutWriter || bw.Buffered() > 0 {
			retained = append(retained, bw)
		} else {
			delete(env.bufioUnderlying, bw)
		}
	}
	env.ActiveBufioWriters = retained
	return firstErr
}

// ResetBufioWritersOnError discards any unflushed buffered data from StdoutWriter and
// resets all custom bufio writers back to their underlying targets while discarding unflushed data.
// Used after evaluation errors or flush failures to prevent abandoned buffers from leaking into subsequent program runs.
func (env *Environment) ResetBufioWritersOnError() {
	if env.parent != nil {
		env.parent.ResetBufioWritersOnError()
		return
	}
	for _, bw := range env.ActiveBufioWriters {
		if bw != env.StdoutWriter {
			if underlying := env.bufioUnderlying[bw]; underlying != nil {
				bw.Reset(underlying)
			} else {
				bw.Reset(io.Discard)
			}
		}
	}
	if env.StdoutWriter != nil {
		env.StdoutWriter.Reset(env.GetRawStdout())
		env.ActiveBufioWriters = []*bufio.Writer{env.StdoutWriter}
	} else {
		env.ActiveBufioWriters = nil
	}
	env.bufioUnderlying = make(map[*bufio.Writer]io.Writer)
}

// ClearCustomBufioWriters is an alias for ResetBufioWritersOnError.
func (env *Environment) ClearCustomBufioWriters() {
	env.ResetBufioWritersOnError()
}
