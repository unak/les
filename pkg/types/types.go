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
	TypeBool   PrimType = "Bool"
	TypeNil    PrimType = "Nil"
	TypeAny    PrimType = "Any"
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
	return fmt.Sprintf("Dict[%s, %s]", d.Key.String(), d.Value.String())
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
	Arg Type // argument is always a TupleType or Any
	Ret Type
}

func (f *FuncType) String() string {
	return fmt.Sprintf("%s -> %s", f.Arg.String(), f.Ret.String())
}

func (f *FuncType) Equals(other Type) bool {
	of, ok := other.(*FuncType)
	if !ok {
		return false
	}
	return f.Arg.Equals(of.Arg) && f.Ret.Equals(of.Ret)
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
		if m == TypeAny {
			return TypeAny
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
	if target == TypeAny {
		return true
	}
	if target.Equals(src) {
		return true
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
	return false
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

	return nil, fmt.Errorf("type %s has no field or method '%s'", s.Name, name)
}

