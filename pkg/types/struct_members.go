package types

import "les/pkg/ast"

// checkDuplicateStructMembers rejects a struct that declares two non-embedded
// fields with the same name, two embedded structs with identical type
// annotations, or two methods with the same name. Only declarations made
// directly in s are compared; members promoted from embedded structs are not
// considered. The error points at the later declaration.
//
// Existing behaviour that is deliberately kept:
//   - A directly declared field may share its name with an embedded struct
//     and shadows it.
//   - Embedded structs that share a simple name but have different type
//     annotations (for example m1::Base and m2::Base, or Box[Int] and
//     Box[String]) are accepted; using the name is an ambiguous selector.
//   - A unary and a binary method of the operators + and - form an overload
//     pair, so +() and +(other) do not conflict. Unary and binary are decided
//     by the same arity test that inferStmt uses to pair them (an explicit
//     self parameter is ignored); a third method of the name is a duplicate.
func (inf *Inferencer) checkDuplicateStructMembers(s *ast.StructDef) error {
	fields := make(map[string]bool, len(s.Fields))
	for _, f := range s.Fields {
		key := f.Name
		if f.IsEmbedded {
			key = "embed " + f.TypeAnnot
		}
		if fields[key] {
			return inf.posErr(f.Position, "duplicate field '%s' in struct '%s'", f.Name, s.Name)
		}
		fields[key] = true
	}
	methods := make(map[string]*methodArity, len(s.Methods))
	for _, m := range s.Methods {
		minA, maxA := methodArityRange(m)
		prev, seen := methods[m.Name]
		if !seen {
			methods[m.Name] = &methodArity{min: minA, max: maxA}
			continue
		}
		if (m.Name == "+" || m.Name == "-") && !prev.paired &&
			((maxA == 0 && prev.min >= 1) || (prev.max == 0 && minA >= 1)) {
			prev.paired = true
			continue
		}
		return inf.nodeErr(m, "duplicate method '%s' in struct '%s'", m.Name, s.Name)
	}
	return nil
}

// methodArity records the arity range of the first method of a name and
// whether a second + or - method has already been paired with it.
type methodArity struct {
	min, max int
	paired   bool
}

// methodArityRange computes MinArity and MaxArity exactly as the StructDef
// case of inferStmt does when it builds the method type: parameters named
// self are ignored, defaults and variadic parameters do not count towards the
// minimum, and a variadic parameter makes the maximum -1.
func methodArityRange(m *ast.FuncDef) (minA, maxA int) {
	variadic := false
	for _, p := range m.Params {
		if p.Name == "self" {
			continue
		}
		maxA++
		if p.DefaultValue == nil && !p.IsVariadic {
			minA++
		}
		if p.IsVariadic {
			variadic = true
		}
	}
	if variadic {
		maxA = -1
	}
	return minA, maxA
}
