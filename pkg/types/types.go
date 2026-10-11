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

// TupleType represents (T1, T2, ...).
type TupleType struct {
	Elems []Type
}

func (t *TupleType) String() string {
	res := "("
	for i, e := range t.Elems {
		if i > 0 {
			res += ", "
		}
		res += e.String()
	}
	res += ")"
	return res
}

func (t *TupleType) Equals(other Type) bool {
	if other == TypeAny {
		return true
	}
	ot, ok := other.(*TupleType)
	if !ok || len(t.Elems) != len(ot.Elems) {
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
