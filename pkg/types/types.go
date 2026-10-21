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

// TypeParamDef defines a type parameter with its name and optional constraint.
type TypeParamDef struct {
	Name       string
	Constraint Type // nil if unconstrained
}

// TypeParamType represents a type parameter inside a generic body or constraint.
type TypeParamType struct {
	Name       string
	Constraint Type // nil if unconstrained
}

func (tp *TypeParamType) String() string {
	return tp.Name
}

func (tp *TypeParamType) Equals(other Type) bool {
	otp, ok := other.(*TypeParamType)
	return ok && tp.Name == otp.Name
}

// TupleType represents (T1, T2, ...) or infinite stream (T1, ...).
type TupleType struct {
	Elems      []Type
	IsVariadic bool // if true, the last element type repeats indefinitely (T, ...)
}

func (t *TupleType) String() string {
	if t.IsVariadic && (len(t.Elems) == 0 || (len(t.Elems) == 1 && t.Elems[0] == TypeUnknown)) {
		return "(...)"
	}
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
	if d.Key == TypeUnknown && d.Value == TypeUnknown {
		return "{...}"
	}
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
	TypeParams    []TypeParamDef
	TypeArgs      []Type
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
	// If src is TypeParamType
	if tpSrc, ok := src.(*TypeParamType); ok {
		if tpTarget, ok := target.(*TypeParamType); ok && tpSrc.Name == tpTarget.Name {
			return true
		}
		if tpSrc.Constraint != nil {
			return IsAssignableTo(tpSrc.Constraint, target)
		}
		return false
	}
	// If target is TypeParamType
	if tpTarget, ok := target.(*TypeParamType); ok {
		if tpSrc, ok := src.(*TypeParamType); ok && tpSrc.Name == tpTarget.Name {
			return true
		}
		return false
	}

	// If target is InterfaceType, check if src implements it
	if ifaceTarget, ok := target.(*InterfaceType); ok {
		if ifaceSrc, ok := src.(*InterfaceType); ok && ifaceTarget.Name == ifaceSrc.Name {
			if len(ifaceTarget.TypeArgs) == 0 {
				return true
			}
			if len(ifaceSrc.TypeArgs) == len(ifaceTarget.TypeArgs) {
				for i := range ifaceSrc.TypeArgs {
					if !IsAssignableTo(ifaceSrc.TypeArgs[i], ifaceTarget.TypeArgs[i]) {
						return false
					}
				}
				return true
			}
			if len(ifaceSrc.TypeArgs) == 0 && len(ifaceSrc.TypeParams) == len(ifaceTarget.TypeArgs) {
				for i, tp := range ifaceSrc.TypeParams {
					tpType := &TypeParamType{Name: tp.Name, Constraint: tp.Constraint}
					if !IsAssignableTo(tpType, ifaceTarget.TypeArgs[i]) {
						return false
					}
				}
				return true
			}
		}
		return ImplementsInterface(src, ifaceTarget)
	}

	// If target is StructType and src is StructType
	if sTarget, ok := target.(*StructType); ok {
		if sSrc, ok := src.(*StructType); ok && sTarget.Name == sSrc.Name {
			if len(sTarget.TypeArgs) == 0 {
				return true
			}
			if len(sSrc.TypeArgs) == len(sTarget.TypeArgs) {
				for i := range sSrc.TypeArgs {
					if !IsAssignableTo(sSrc.TypeArgs[i], sTarget.TypeArgs[i]) {
						return false
					}
				}
				return true
			}
			if len(sSrc.TypeArgs) == 0 && len(sSrc.TypeParams) == len(sTarget.TypeArgs) {
				for i, tp := range sSrc.TypeParams {
					tpType := &TypeParamType{Name: tp.Name, Constraint: tp.Constraint}
					if !IsAssignableTo(tpType, sTarget.TypeArgs[i]) {
						return false
					}
				}
				return true
			}
		}
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
	Name          string
	TypeParams    []TypeParamDef
	TypeArgs      []Type
	GenericOrigin *InterfaceType
	Methods       map[string]*FuncType
	Embedded      []*InterfaceType
	instCache     map[string]*InterfaceType
}

func (i *InterfaceType) ClearInstCache() {
	i.instCache = nil
}

func (i *InterfaceType) String() string {
	if len(i.TypeArgs) > 0 {
		var args []string
		for _, a := range i.TypeArgs {
			args = append(args, a.String())
		}
		return fmt.Sprintf("%s[%s]", i.Name, strings.Join(args, ", "))
	}
	return i.Name
}

func (i *InterfaceType) Equals(other Type) bool {
	oi, ok := other.(*InterfaceType)
	if !ok || i.Name != oi.Name {
		return false
	}
	if len(i.TypeArgs) != len(oi.TypeArgs) {
		return false
	}
	for j := range i.TypeArgs {
		if !i.TypeArgs[j].Equals(oi.TypeArgs[j]) {
			return false
		}
	}
	return true
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
	if tpSrc, ok := src.(*TypeParamType); ok {
		if tpSrc.Constraint != nil {
			return ImplementsInterface(tpSrc.Constraint, target)
		}
		return false
	}

	// If src is itself an interface, check if it contains all required methods
	if srcIface, ok := src.(*InterfaceType); ok {
		if target.Name == srcIface.Name {
			if len(target.TypeArgs) == 0 {
				return true
			}
			if len(srcIface.TypeArgs) == len(target.TypeArgs) {
				for j := range srcIface.TypeArgs {
					if !IsAssignableTo(srcIface.TypeArgs[j], target.TypeArgs[j]) {
						return false
					}
				}
				return true
			}
		}
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
		matched := targetSig.Equals(srcSig)
		if !matched && len(srcSig.Overloads) > 0 {
			for _, ov := range srcSig.Overloads {
				if targetSig.Equals(ov) {
					matched = true
					break
				}
			}
		}
		if !matched && (srcSig.MinArity > 0 || srcSig.MaxArity > 0) {
			if targetTup, ok := targetSig.Arg.(*TupleType); ok {
				targetCount := len(targetTup.Elems)
				if targetCount >= srcSig.MinArgs() && (srcSig.MaxArgs() < 0 || targetCount <= srcSig.MaxArgs()) {
					if targetSig.Ret == TypeUnknown || srcSig.Ret == TypeUnknown || targetSig.Ret.Equals(srcSig.Ret) {
						argsMatch := true
						if srcTup, ok := srcSig.Arg.(*TupleType); ok {
							for i := 0; i < targetCount && i < len(srcTup.Elems); i++ {
								if targetTup.Elems[i] != TypeUnknown && srcTup.Elems[i] != TypeUnknown && !targetTup.Elems[i].Equals(srcTup.Elems[i]) {
									argsMatch = false
									break
								}
							}
						}
						if argsMatch {
							matched = true
						}
					}
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// LookupBuiltinTypeMethod returns the method signature if type t has a built-in method named mName.
func LookupBuiltinTypeMethod(t Type, mName string) (*FuncType, bool) {
	if tp, ok := t.(*TypeParamType); ok {
		if tp.Constraint != nil {
			if iface, ok := tp.Constraint.(*InterfaceType); ok {
				return iface.LookupMethod(mName)
			}
			if st, ok := tp.Constraint.(*StructType); ok {
				if m, err := st.LookupMember(mName); err == nil {
					if fnT, ok := m.(*FuncType); ok {
						return fnT, true
					}
				}
			}
			return LookupBuiltinTypeMethod(tp.Constraint, mName)
		}
		if mName == "Type" || mName == "ToString" {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			}, true
		}
		return nil, false
	}

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
				Overloads: []*FuncType{
					{Arg: &TupleType{Elems: []Type{}}, Ret: TypeInt},
					{Arg: &TupleType{Elems: []Type{TypeInt}}, Ret: TypeInt},
				},
				MinArity: 0,
				MaxArity: 1,
				Ret:      TypeInt,
			}, true
		}
		if t == TypeFloat {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeFloat}},
				OverloadUnion: "() -> Float | (Float) -> Float",
				Overloads: []*FuncType{
					{Arg: &TupleType{Elems: []Type{}}, Ret: TypeFloat},
					{Arg: &TupleType{Elems: []Type{TypeFloat}}, Ret: TypeFloat},
				},
				MinArity: 0,
				MaxArity: 1,
				Ret:      TypeFloat,
			}, true
		}
		if t == TypeString {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeString}},
				Ret: TypeString,
			}, true
		}
		if tt, ok := t.(*TupleType); ok {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{&TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true}}},
				Ret: &TupleType{Elems: append([]Type(nil), tt.Elems...), IsVariadic: true},
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
				Overloads: []*FuncType{
					{Arg: &TupleType{Elems: []Type{}}, Ret: TypeInt},
					{Arg: &TupleType{Elems: []Type{TypeInt}}, Ret: TypeInt},
				},
				MinArity: 0,
				MaxArity: 1,
				Ret:      TypeInt,
			}, true
		}
		if t == TypeFloat {
			return &FuncType{
				Arg:           &TupleType{Elems: []Type{TypeFloat}},
				OverloadUnion: "() -> Float | (Float) -> Float",
				Overloads: []*FuncType{
					{Arg: &TupleType{Elems: []Type{}}, Ret: TypeFloat},
					{Arg: &TupleType{Elems: []Type{TypeFloat}}, Ret: TypeFloat},
				},
				MinArity: 0,
				MaxArity: 1,
				Ret:      TypeFloat,
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
	case "~":
		if t == TypeInt {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeInt,
			}, true
		}
	case "&", "|", "^", ">>":
		if t == TypeInt {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: TypeInt,
			}, true
		}
	case "<<":
		if t == TypeInt {
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: TypeInt,
			}, true
		}
		if tt, ok := t.(*TupleType); ok {
			var retT Type
			if tt.IsVariadic {
				retT = tt
			} else {
				newElems := append(append([]Type(nil), tt.Elems...), TypeUnknown)
				retT = &TupleType{Elems: newElems}
			}
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeUnknown}},
				Ret: retT,
			}, true
		}
	case "Append":
		if tt, ok := t.(*TupleType); ok {
			var retT Type
			if tt.IsVariadic {
				retT = tt
			} else {
				newElems := append(append([]Type(nil), tt.Elems...), TypeUnknown)
				retT = &TupleType{Elems: newElems}
			}
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeUnknown}},
				Ret: retT,
			}, true
		}
	case "Prepend":
		if tt, ok := t.(*TupleType); ok {
			newElems := append([]Type{TypeUnknown}, tt.Elems...)
			return &FuncType{
				Arg: &TupleType{Elems: []Type{TypeUnknown}},
				Ret: &TupleType{Elems: newElems, IsVariadic: tt.IsVariadic},
			}, true
		}
	case "Concat":
		if tt, ok := t.(*TupleType); ok {
			return &FuncType{
				Arg:      &TupleType{Elems: []Type{&TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true}}, IsVariadic: true},
				MinArity: 1,
				MaxArity: -1,
				Ret:      &TupleType{Elems: append([]Type(nil), tt.Elems...), IsVariadic: true},
			}, true
		}
	case "Slice":
		if tt, ok := t.(*TupleType); ok {
			return &FuncType{
				Arg:      &TupleType{Elems: []Type{TypeInt, TypeInt}},
				MinArity: 1,
				MaxArity: 2,
				Ret:      &TupleType{Elems: append([]Type(nil), tt.Elems...), IsVariadic: true},
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
	Name          string
	TypeParams    []TypeParamDef
	TypeArgs      []Type
	GenericOrigin *StructType
	Fields        map[string]Type
	FieldOrder    []string
	Embedded      []*StructType
	Methods       map[string]*FuncType
	instCache     map[string]*StructType
}

func (s *StructType) ClearInstCache() {
	s.instCache = nil
}

func (s *StructType) String() string {
	if len(s.TypeArgs) > 0 {
		var args []string
		for _, a := range s.TypeArgs {
			args = append(args, a.String())
		}
		return fmt.Sprintf("%s[%s]", s.Name, strings.Join(args, ", "))
	}
	return s.Name
}

func (s *StructType) Equals(other Type) bool {
	os, ok := other.(*StructType)
	if !ok || s.Name != os.Name {
		return false
	}
	if len(s.TypeArgs) != len(os.TypeArgs) {
		return false
	}
	for i := range s.TypeArgs {
		if !s.TypeArgs[i].Equals(os.TypeArgs[i]) {
			return false
		}
	}
	return true
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

// Substitute replaces all TypeParamType occurrences in t with bound types in bindings.
func Substitute(t Type, bindings map[string]Type) Type {
	if t == nil {
		return nil
	}
	switch typ := t.(type) {
	case *TypeParamType:
		if bound, ok := bindings[typ.Name]; ok {
			return bound
		}
		return typ
	case *TupleType:
		newElems := make([]Type, len(typ.Elems))
		for i, el := range typ.Elems {
			newElems[i] = Substitute(el, bindings)
		}
		return &TupleType{Elems: newElems, IsVariadic: typ.IsVariadic}
	case *DictType:
		return &DictType{
			Key:   Substitute(typ.Key, bindings),
			Value: Substitute(typ.Value, bindings),
		}
	case *FuncType:
		newF := &FuncType{
			TypeParams:    typ.TypeParams,
			TypeArgs:      typ.TypeArgs,
			Arg:           Substitute(typ.Arg, bindings),
			OverloadUnion: typ.OverloadUnion,
			MinArity:      typ.MinArity,
			MaxArity:      typ.MaxArity,
			BlockParam:    Substitute(typ.BlockParam, bindings),
			BlockOpt:      typ.BlockOpt,
			Ret:           Substitute(typ.Ret, bindings),
		}
		if len(typ.Overloads) > 0 {
			newF.Overloads = make([]*FuncType, len(typ.Overloads))
			for i, ov := range typ.Overloads {
				newF.Overloads[i] = Substitute(ov, bindings).(*FuncType)
			}
			if len(newF.Overloads) == 2 && typ.OverloadUnion != "" {
				newF.OverloadUnion = fmt.Sprintf("%s | %s", newF.Overloads[0].String(), newF.Overloads[1].String())
			}
		}
		return newF
	case *UnionType:
		newMembers := make([]Type, len(typ.Members))
		for i, m := range typ.Members {
			newMembers[i] = Substitute(m, bindings)
		}
		return NewUnionType(newMembers...)
	case *StructType:
		if len(typ.TypeArgs) > 0 {
			newArgs := make([]Type, len(typ.TypeArgs))
			for i, a := range typ.TypeArgs {
				newArgs[i] = Substitute(a, bindings)
			}
			origin := typ.GenericOrigin
			if origin == nil {
				origin = typ
			}
			return InstantiateStruct(origin, newArgs)
		}
		if len(typ.TypeParams) > 0 {
			newArgs := make([]Type, len(typ.TypeParams))
			allBound := true
			for i, tp := range typ.TypeParams {
				if b, ok := bindings[tp.Name]; ok {
					newArgs[i] = b
				} else {
					allBound = false
					break
				}
			}
			if allBound {
				return InstantiateStruct(typ, newArgs)
			}
		}
		return typ
	case *InterfaceType:
		if len(typ.TypeArgs) > 0 {
			newArgs := make([]Type, len(typ.TypeArgs))
			for i, a := range typ.TypeArgs {
				newArgs[i] = Substitute(a, bindings)
			}
			origin := typ.GenericOrigin
			if origin == nil {
				origin = typ
			}
			return InstantiateInterface(origin, newArgs)
		}
		if len(typ.TypeParams) > 0 {
			newArgs := make([]Type, len(typ.TypeParams))
			allBound := true
			for i, tp := range typ.TypeParams {
				if b, ok := bindings[tp.Name]; ok {
					newArgs[i] = b
				} else {
					allBound = false
					break
				}
			}
			if allBound {
				return InstantiateInterface(typ, newArgs)
			}
		}
		return typ
	default:
		return t
	}
}

// InstantiateStruct specializes a generic StructType with concrete type arguments.
func InstantiateStruct(s *StructType, typeArgs []Type) *StructType {
	if s == nil || len(typeArgs) == 0 {
		return s
	}
	origin := s.GenericOrigin
	if origin == nil {
		origin = s
	}
	var argStrs []string
	for _, a := range typeArgs {
		argStrs = append(argStrs, a.String())
	}
	cacheKey := strings.Join(argStrs, ", ")
	if origin.instCache != nil {
		if cached, ok := origin.instCache[cacheKey]; ok {
			return cached
		}
	}

	inst := &StructType{
		Name:          origin.Name,
		TypeParams:    origin.TypeParams,
		TypeArgs:      typeArgs,
		GenericOrigin: origin,
		FieldOrder:    origin.FieldOrder,
		Fields:        make(map[string]Type),
		Methods:       make(map[string]*FuncType),
	}
	if origin.instCache == nil {
		origin.instCache = make(map[string]*StructType)
	}
	origin.instCache[cacheKey] = inst

	bindings := make(map[string]Type)
	for i, tp := range origin.TypeParams {
		if i < len(typeArgs) {
			bindings[tp.Name] = typeArgs[i]
		}
	}

	for fName, fType := range origin.Fields {
		inst.Fields[fName] = Substitute(fType, bindings)
	}
	for mName, mType := range origin.Methods {
		inst.Methods[mName] = Substitute(mType, bindings).(*FuncType)
	}
	for _, emb := range origin.Embedded {
		inst.Embedded = append(inst.Embedded, Substitute(emb, bindings).(*StructType))
	}

	return inst
}

// InstantiateInterface specializes a generic InterfaceType with concrete type arguments.
func InstantiateInterface(i *InterfaceType, typeArgs []Type) *InterfaceType {
	if i == nil || len(typeArgs) == 0 {
		return i
	}
	origin := i.GenericOrigin
	if origin == nil {
		origin = i
	}
	var argStrs []string
	for _, a := range typeArgs {
		argStrs = append(argStrs, a.String())
	}
	cacheKey := strings.Join(argStrs, ", ")
	if origin.instCache != nil {
		if cached, ok := origin.instCache[cacheKey]; ok {
			return cached
		}
	}

	inst := &InterfaceType{
		Name:          origin.Name,
		TypeParams:    origin.TypeParams,
		TypeArgs:      typeArgs,
		GenericOrigin: origin,
		Methods:       make(map[string]*FuncType),
	}
	if origin.instCache == nil {
		origin.instCache = make(map[string]*InterfaceType)
	}
	origin.instCache[cacheKey] = inst

	bindings := make(map[string]Type)
	for j, tp := range origin.TypeParams {
		if j < len(typeArgs) {
			bindings[tp.Name] = typeArgs[j]
		}
	}

	for mName, mType := range origin.Methods {
		inst.Methods[mName] = Substitute(mType, bindings).(*FuncType)
	}
	for _, emb := range origin.Embedded {
		inst.Embedded = append(inst.Embedded, Substitute(emb, bindings).(*InterfaceType))
	}

	return inst
}

// ValidateTypeConstraints validates type arguments against type parameter constraints.
func ValidateTypeConstraints(typeParams []TypeParamDef, typeArgs []Type) error {
	if len(typeParams) != len(typeArgs) {
		return fmt.Errorf("expected %d type arguments, got %d", len(typeParams), len(typeArgs))
	}
	bindings := make(map[string]Type)
	for i, tp := range typeParams {
		bindings[tp.Name] = typeArgs[i]
	}
	for i, tp := range typeParams {
		if tp.Constraint != nil {
			subConstraint := Substitute(tp.Constraint, bindings)
			if !IsAssignableTo(typeArgs[i], subConstraint) {
				return fmt.Errorf("type %s does not satisfy constraint %s for type parameter %s", typeArgs[i].String(), subConstraint.String(), tp.Name)
			}
		}
	}
	return nil
}
