package types

import (
	"fmt"
	"strings"
)

// Type represents a Les static type.
type Type interface {
	String() string
	Equals(other Type) bool
}

// Primitive types
type PrimType string

const (
	TypeInt    PrimType = "Int"
	TypeFloat  PrimType = "Float"
	TypeString PrimType = "String"
	TypeBytes  PrimType = "Bytes"
	TypeBool    PrimType = "Bool"
	TypeNil     PrimType = "Nil"
	TypeUnknown PrimType = "Unknown"
)

func (p PrimType) String() string { return string(p) }
func (p PrimType) Equals(other Type) bool {
	op, ok := other.(PrimType)
	return ok && p == op
}

// TupleType represents (T1, T2, ...) or infinite stream (T1, ...).
type TupleType struct {
	Elems      []Type
	IsVariadic bool // if true, the last element type repeats indefinitely (T, ...)
}

func (t *TupleType) String() string {
	res := "("
	for i, e := range t.Elems {
		if i > 0 {
			res += ", "
		}
		res += e.String()
	}
	if t.IsVariadic {
		if len(t.Elems) > 0 {
			res += ", ..."
		} else {
			res += "..."
		}
	}
	res += ")"
	return res
}

func (t *TupleType) Equals(other Type) bool {
	ot, ok := other.(*TupleType)
	if !ok {
		return false
	}
	if t.IsVariadic {
		lastIdx := len(t.Elems) - 1
		for i, el := range ot.Elems {
			expected := t.Elems[lastIdx]
			if i < len(t.Elems) {
				expected = t.Elems[i]
			}
			if !expected.Equals(el) {
				return false
			}
		}
		return true
	}
	if len(t.Elems) != len(ot.Elems) {
		return false
	}
	for i := range t.Elems {
		if !t.Elems[i].Equals(ot.Elems[i]) {
			return false
		}
	}
	return true
}

// DictType represents Dict of Key -> Value.
type DictType struct {
	Key   Type
	Value Type
}

func (d *DictType) String() string {
	return fmt.Sprintf("{%s: %s}", d.Key.String(), d.Value.String())
}

func (d *DictType) Equals(other Type) bool {
	od, ok := other.(*DictType)
	if !ok {
		return false
	}
	return d.Key.Equals(od.Key) && d.Value.Equals(od.Value)
}

// FuncType represents (Arg) -> Ret.
type FuncType struct {
	Arg           Type        // argument is always a TupleType or TypeUnknown
	OverloadUnion string      // if set, custom string representation for overloaded methods
	Overloads     []*FuncType // if overloaded, individual arity-specific signatures
	MinArity      int         // Minimum number of required positional arguments
	MaxArity      int         // Maximum number of positional arguments (-1 if variadic)
	BlockParam    Type        // Type of &block parameter (nil if no &block)
	BlockOpt      bool        // true if &block is optional (= nil)
	Ret           Type
}

func (f *FuncType) MinArgs() int {
	if f.MinArity > 0 || (f.Arg != nil && f.MaxArity > 0) {
		return f.MinArity
	}
	if tup, ok := f.Arg.(*TupleType); ok {
		if tup.IsVariadic && len(tup.Elems) > 0 {
			return len(tup.Elems) - 1
		}
		return len(tup.Elems)
	}
	return 0
}

func (f *FuncType) MaxArgs() int {
	if f.MaxArity != 0 {
		return f.MaxArity
	}
	if tup, ok := f.Arg.(*TupleType); ok {
		if tup.IsVariadic {
			return -1
		}
		return len(tup.Elems)
	}
	return -1
}

func (f *FuncType) String() string {
	if f.OverloadUnion != "" {
		return f.OverloadUnion
	}
	argStr := f.Arg.String()
	if f.BlockParam != nil {
		bpStr := f.BlockParam.String()
		if tup, ok := f.Arg.(*TupleType); ok {
			var parts []string
			for _, el := range tup.Elems {
				parts = append(parts, el.String())
			}
			if tup.IsVariadic {
				parts = append(parts, "...")
			}
			parts = append(parts, bpStr)
			argStr = "(" + strings.Join(parts, ", ") + ")"
		} else {
			argStr = fmt.Sprintf("(%s, %s)", f.Arg.String(), bpStr)
		}
	}
	return fmt.Sprintf("%s -> %s", argStr, f.Ret.String())
}

func (f *FuncType) effectiveArgType() Type {
	if f.BlockParam == nil {
		return f.Arg
	}
	if tup, ok := f.Arg.(*TupleType); ok {
		elems := make([]Type, len(tup.Elems)+1)
		copy(elems, tup.Elems)
		elems[len(tup.Elems)] = f.BlockParam
		return &TupleType{
			Elems:      elems,
			IsVariadic: tup.IsVariadic,
		}
	}
	return &TupleType{
		Elems: []Type{f.Arg, f.BlockParam},
	}
}

func (f *FuncType) Equals(other Type) bool {
	of, ok := other.(*FuncType)
	if !ok {
		return false
	}
	fArg := f.effectiveArgType()
	oArg := of.effectiveArgType()
	argEq := fArg == TypeUnknown || oArg == TypeUnknown || fArg.Equals(oArg)
	if !argEq {
		fTup, fOk := fArg.(*TupleType)
		oTup, oOk := oArg.(*TupleType)
		if fOk && oOk && !fTup.IsVariadic && !oTup.IsVariadic && len(fTup.Elems) == len(oTup.Elems) {
			argEq = true
			for i := range fTup.Elems {
				if fTup.Elems[i] != TypeUnknown && oTup.Elems[i] != TypeUnknown && !fTup.Elems[i].Equals(oTup.Elems[i]) {
					argEq = false
					break
				}
			}
		}
	}
	retEq := f.Ret == TypeUnknown || of.Ret == TypeUnknown || f.Ret.Equals(of.Ret)
	return argEq && retEq
}




// TypeVar represents uninstantiated type variable for inference.
type TypeVar struct {
	ID int
}

func (tv *TypeVar) String() string {
	return fmt.Sprintf("t%d", tv.ID)
}

func (tv *TypeVar) Equals(other Type) bool {
	otv, ok := other.(*TypeVar)
	return ok && tv.ID == otv.ID
}

// UnionType represents a union of types A | B | C.
type UnionType struct {
	Members []Type
}

func NewUnionType(members ...Type) Type {
	var flat []Type
	for _, m := range members {
		if u, ok := m.(*UnionType); ok {
			flat = append(flat, u.Members...)
		} else {
			flat = append(flat, m)
		}
	}
	// Deduplicate
	var unique []Type
	for _, m := range flat {
		if m == TypeUnknown {
			return TypeUnknown
		}
		exists := false
		for _, u := range unique {
			if u.Equals(m) {
				exists = true
				break
			}
		}
		if !exists {
			unique = append(unique, m)
		}
	}
	if len(unique) == 1 {
		return unique[0]
	}
	return &UnionType{Members: unique}
}

func (u *UnionType) String() string {
	var res string
	for i, m := range u.Members {
		if i > 0 {
			res += " | "
		}
		res += m.String()
	}
	return res
}

func (u *UnionType) Equals(other Type) bool {
	ou, ok := other.(*UnionType)
	if !ok {
		return false
	}
	if len(u.Members) != len(ou.Members) {
		return false
	}
	for _, m := range u.Members {
		found := false
		for _, om := range ou.Members {
			if m.Equals(om) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsAssignableTo checks if source type can be assigned to target type (handling subtyping and unions).
func IsAssignableTo(src, target Type) bool {
	if src == TypeUnknown || target == TypeUnknown {
		return true
	}
	if target.Equals(src) {
		return true
	}
	// If target is InterfaceType, check if src implements it
	if ifaceTarget, ok := target.(*InterfaceType); ok {
		return ImplementsInterface(src, ifaceTarget)
	}

	// If target is UnionType, src must match at least one member
	if uTarget, ok := target.(*UnionType); ok {
		// If src is also UnionType, every member of src must be assignable to target
		if uSrc, ok := src.(*UnionType); ok {
			for _, sm := range uSrc.Members {
				if !IsAssignableTo(sm, target) {
					return false
				}
			}
			return true
		}
		for _, m := range uTarget.Members {
			if IsAssignableTo(src, m) {
				return true
			}
		}
		return false
	}

	// If both are FuncType
	if fSrc, ok := src.(*FuncType); ok {
		if fTarget, ok := target.(*FuncType); ok {
			srcArgs, sIsTuple := fSrc.effectiveArgType().(*TupleType)
			targetArgs, tIsTuple := fTarget.effectiveArgType().(*TupleType)
			if sIsTuple && tIsTuple {
				if len(srcArgs.Elems) != len(targetArgs.Elems) {
					return false
				}
				for i := range srcArgs.Elems {
					// Contravariant parameter types
					if !IsAssignableTo(targetArgs.Elems[i], srcArgs.Elems[i]) {
						return false
					}
				}
			} else {
				if !IsAssignableTo(fTarget.effectiveArgType(), fSrc.effectiveArgType()) {
					return false
				}
			}
			// Covariant return type
			return IsAssignableTo(fSrc.Ret, fTarget.Ret)
		}
	}

	// If both are DictType
	if dTarget, ok := target.(*DictType); ok {
		if dSrc, ok := src.(*DictType); ok {
			keyOK := dSrc.Key == TypeUnknown || IsAssignableTo(dSrc.Key, dTarget.Key)
			valOK := dSrc.Value == TypeUnknown || IsAssignableTo(dSrc.Value, dTarget.Value)
			return keyOK && valOK
		}
		return false
	}

	// If both are TupleType
	if tTarget, ok := target.(*TupleType); ok {
		if tSrc, ok := src.(*TupleType); ok {
			if tTarget.IsVariadic {
				lastIdx := len(tTarget.Elems) - 1
				for i, el := range tSrc.Elems {
					var expected Type = TypeUnknown
					if lastIdx >= 0 {
						if i < len(tTarget.Elems) {
							expected = tTarget.Elems[i]
						} else {
							expected = tTarget.Elems[lastIdx]
						}
					}
					if !IsAssignableTo(el, expected) {
						return false
					}
				}
				return true
			}
			if len(tSrc.Elems) != len(tTarget.Elems) {
				return false
			}
			for i := range tSrc.Elems {
				if !IsAssignableTo(tSrc.Elems[i], tTarget.Elems[i]) {
					return false
				}
			}
			return true
		}
		return false
	}

	return false
}

// InterfaceType represents an interface type with method signatures.
type InterfaceType struct {
	Name     string
	Methods  map[string]*FuncType
	Embedded []*InterfaceType
}

func (i *InterfaceType) String() string {
	return i.Name
}

func (i *InterfaceType) Equals(other Type) bool {
	oi, ok := other.(*InterfaceType)
	return ok && i.Name == oi.Name
}

func (i *InterfaceType) LookupMethod(name string) (*FuncType, bool) {
	if m, ok := i.Methods[name]; ok {
		return m, true
	}
	if name == "Type" {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		}, true
	}
	return nil, false
}

// ImplementsInterface checks if src type satisfies target interface.
func ImplementsInterface(src Type, target *InterfaceType) bool {
	// If src is itself an interface, check if it contains all required methods
	if srcIface, ok := src.(*InterfaceType); ok {
		for mName, targetSig := range target.Methods {
			srcSig, ok := srcIface.LookupMethod(mName)
			if !ok {
				return false
			}
			if !targetSig.Equals(srcSig) {
				return false
			}
		}
		return true
	}

	// If src is a StructType, check all methods via LookupMember
	if srcStruct, ok := src.(*StructType); ok {
		for mName, targetSig := range target.Methods {
			memberT, err := srcStruct.LookupMember(mName)
			if err != nil {
				return false
			}
			fnT, ok := memberT.(*FuncType)
			if !ok {
				return false
			}
			matched := targetSig.Equals(fnT)
			if !matched && len(fnT.Overloads) > 0 {
				for _, ov := range fnT.Overloads {
					if targetSig.Equals(ov) {
						matched = true
						break
					}
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}

	// For other types (e.g. primitives, tuples, dicts), check if built-in methods satisfy target
	if len(target.Methods) == 0 {
		return true
	}

	for mName, targetSig := range target.Methods {
		srcSig, ok := LookupBuiltinTypeMethod(src, mName)
		if !ok {
			return false
		}
		if (target.Name == "Equal" || target.Name == "Comparable") && (mName == "==" || mName == "<") {
			// For Equal/Comparable, builtin method must accept 1 argument and return Bool
			if srcSig.Ret != TypeBool {
				return false
			}
			tup, ok := srcSig.Arg.(*TupleType)
			if !ok || len(tup.Elems) != 1 {
				return false
			}
			continue
		}
		if !targetSig.Equals(srcSig) {
			return false
		}
	}
	return true
}

// LookupBuiltinTypeMethod returns the method signature if type t has a built-in method named mName.
func LookupBuiltinTypeMethod(t Type, mName string) (*FuncType, bool) {
	if mName == "ToString" {
		switch t.(type) {
		case PrimType, *TupleType, *DictType, *FuncType, *TypeVar, *UnionType:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			}, true
		}
	}
	if mName == "Type" {
		switch t.(type) {
		case PrimType, *TupleType, *DictType, *FuncType, *TypeVar, *UnionType:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			}, true
		}
	}

	if mName == "Len" {
		switch t {
		case TypeString, TypeBytes:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeInt,
			}, true
		}
		switch t.(type) {
		case *TupleType, *DictType:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeInt,
			}, true
		}
	}
	if u, ok := t.(*UnionType); ok && len(u.Members) > 0 {
		var commonSig *FuncType
		for _, m := range u.Members {
			sig, ok := LookupBuiltinTypeMethod(m, mName)
			if !ok {
				return nil, false
			}
			if commonSig == nil {
				commonSig = sig
			} else if !commonSig.Equals(sig) {
				return nil, false
			}
		}
		return commonSig, true
	}
	if mName == "Ord" && t == TypeString {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeInt,
		}, true
	}
	if mName == "Chr" && t == TypeInt {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		}, true
	}
	if mName == "Bytes" && (t == TypeString || t == TypeBytes) {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeBytes,
		}, true
	}
	if mName == "String" && t == TypeBytes {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		}, true
	}
	if mName == "IsValidString" && t == TypeBytes {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeBool,
		}, true
	}
	if mName == "Int" && t == TypeString {
		return &FuncType{
			Arg:      &TupleType{Elems: []Type{TypeInt}},
			MinArity: 0,
			MaxArity: 1,
			Ret:      TypeInt,
		}, true
	}
	if mName == "String" && t == TypeInt {
		return &FuncType{
			Arg:      &TupleType{Elems: []Type{TypeInt}},
			MinArity: 0,
			MaxArity: 1,
			Ret:      TypeString,
		}, true
	}

	// Built-in operator methods
	switch mName {
	case "==":
		switch t {
		case TypeInt, TypeFloat, TypeString, TypeBytes, TypeBool, TypeNil:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{t}},
				Ret: TypeBool,
			}, true
		}
		switch t.(type) {
		case *TupleType, *DictType:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{t}},
				Ret: TypeBool,
			}, true
		}
	case "<", "<=", ">", ">=":
		switch t {
		case TypeInt, TypeFloat, TypeString:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{t}},
				Ret: TypeBool,
			}, true
		case TypeBytes:
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeBool,
			}, true
		}
	case "+":
		if t == TypeInt {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeInt}},
				OverloadUnion: "() -> Int | (Int) -> Int",
				MinArity:      0,
				MaxArity:      1,
				Ret:           TypeInt,
			}, true
		}
		if t == TypeFloat {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeFloat}},
				OverloadUnion: "() -> Float | (Float) -> Float",
				MinArity:      0,
				MaxArity:      1,
				Ret:           TypeFloat,
			}, true
		}
		if t == TypeString {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeString}},
				Ret: TypeString,
			}, true
		}
		if t == TypeBytes {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeBytes,
			}, true
		}
	case "-":
		if t == TypeInt {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeInt}},
				OverloadUnion: "() -> Int | (Int) -> Int",
				MinArity:      0,
				MaxArity:      1,
				Ret:           TypeInt,
			}, true
		}
		if t == TypeFloat {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeFloat}},
				OverloadUnion: "() -> Float | (Float) -> Float",
				MinArity:      0,
				MaxArity:      1,
				Ret:           TypeFloat,
			}, true
		}
	case "*", "/", "%":
		if t == TypeInt {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: TypeInt,
			}, true
		}
		if t == TypeFloat && (mName == "*" || mName == "/") {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeFloat}},
				Ret: TypeFloat,
			}, true
		}
	case "[]":
		if t == TypeString {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: TypeString,
			}, true
		}
		if t == TypeBytes {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: TypeInt,
			}, true
		}
		if tt, ok := t.(*TupleType); ok {
			var elemT Type = TypeUnknown
			if len(tt.Elems) > 0 {
				elemT = tt.Elems[0]
			}
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: elemT,
			}, true
		}
		if dt, ok := t.(*DictType); ok {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{dt.Key}},
				Ret: dt.Value,
			}, true
		}
	}
	return nil, false
}

// StructFieldInfo holds type info of a struct field.
type StructFieldInfo struct {
	Name       string
	Type       Type
	IsEmbedded bool
}

// StructType represents user-defined struct type with fields, embedded structs, and methods.
type StructType struct {
	Name       string
	Fields     map[string]Type
	FieldOrder []string
	Embedded   []*StructType
	Methods    map[string]*FuncType
}

func (s *StructType) String() string {
	return s.Name
}

func (s *StructType) Equals(other Type) bool {
	os, ok := other.(*StructType)
	return ok && s.Name == os.Name
}

type memberCandidate struct {
	typ    Type
	origin string
}

// LookupMember searches for field or method on StructType with Go-style promotion rules.
// Returns member type, or error if not found or ambiguous.
func (s *StructType) LookupMember(name string) (Type, error) {
	// 1. Direct fields & methods on self (depth 0)
	if ft, ok := s.Fields[name]; ok {
		return ft, nil
	}
	if mt, ok := s.Methods[name]; ok {
		return mt, nil
	}

	// 2. BFS on embedded structs depth by depth
	currentLevel := s.Embedded
	visited := make(map[string]bool)
	visited[s.Name] = true

	for len(currentLevel) > 0 {
		var candidates []memberCandidate
		var nextLevel []*StructType

		for _, emb := range currentLevel {
			if visited[emb.Name] {
				continue
			}
			visited[emb.Name] = true

			if ft, ok := emb.Fields[name]; ok {
				candidates = append(candidates, memberCandidate{typ: ft, origin: emb.Name})
			}
			if mt, ok := emb.Methods[name]; ok {
				candidates = append(candidates, memberCandidate{typ: mt, origin: emb.Name})
			}
			nextLevel = append(nextLevel, emb.Embedded...)
		}

		if len(candidates) == 1 {
			return candidates[0].typ, nil
		}
		if len(candidates) > 1 {
			origins := make([]string, len(candidates))
			for i, c := range candidates {
				origins[i] = c.origin
			}
			return nil, fmt.Errorf("ambiguous selector '%s' on type %s (candidates in %s)", name, s.Name, strings.Join(origins, ", "))
		}

		currentLevel = nextLevel
	}

	if name == "Type" {
		return &FuncType{
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		}, nil
	}

	return nil, fmt.Errorf("type %s has no field or method '%s'", s.Name, name)
}

// ModuleType represents a loaded module and its exported member types and type definitions.
type ModuleType struct {
	Name    string
	Exports map[string]Type // values/functions/constructors accessible via @mod.Member
	Types   map[string]Type // types accessible via @mod.Type in type annotations
}

func (m *ModuleType) String() string { return "Module<" + m.Name + ">" }
func (m *ModuleType) Equals(other Type) bool {
	om, ok := other.(*ModuleType)
	return ok && m.Name == om.Name
}
