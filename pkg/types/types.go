package types

import "fmt"

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
	TypeAny    PrimType = "Any"
)

func (p PrimType) String() string { return string(p) }
func (p PrimType) Equals(other Type) bool {
	if other == TypeAny || p == TypeAny {
		return true
	}
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
	if other == TypeAny {
		return true
	}
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
	if other == TypeAny {
		return true
	}
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
	if other == TypeAny {
		return true
	}
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
	if other == TypeAny {
		return true
	}
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
	if target == TypeAny || src == TypeAny {
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
