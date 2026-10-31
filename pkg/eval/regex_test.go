package eval

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func runEvalCode(t *testing.T, input string) (*Environment, *Evaluator) {
	t.Helper()
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	ev := NewEvaluator()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	return ev.globalEnv, ev
}

func TestRegexAndMatchEvaluation(t *testing.T) {
	input := `
r = /foo/i
s = "FooBar"
m = s =~ r

p = r.Pattern()
f = r.Flags()
matched = r.Test("foo")
not_matched = r.Test("baz")

rep = /a/.Replace("banana", "o")
repAll = /a/.ReplaceAll("banana", "o")
parts = /,/.Split("1,2,3")
findall = /\w+/.FindAll("hello world")

t0 = m[0]
tText = m.Text()
tTarget = m.Target()
tSpan = m.Span(0)
`
	env, _ := runEvalCode(t, input)

	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}

	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}

	checkStr("p", "foo")
	checkStr("f", "i")
	checkBool("matched", true)
	checkBool("not_matched", false)
	checkStr("rep", "bonana")
	checkStr("repAll", "bonono")
	checkStr("t0", "Foo")
	checkStr("tText", "Foo")
	checkStr("tTarget", "FooBar")

	// parts
	partsVal, _ := env.Get("parts")
	partsTuple, ok := Force(partsVal).(*TupleValue)
	if !ok || len(partsTuple.Elements) != 3 {
		t.Fatalf("expected 3 parts, got %v", partsVal)
	}

	// findall
	faVal, _ := env.Get("findall")
	faTuple, ok := Force(faVal).(*TupleValue)
	if !ok || len(faTuple.Elements) != 2 {
		t.Fatalf("expected 2 matches in findall, got %v", faVal)
	}

	// span
	spanVal, _ := env.Get("tSpan")
	spanTuple, ok := Force(spanVal).(*TupleValue)
	if !ok || len(spanTuple.Elements) != 2 {
		t.Fatalf("expected span tuple (0, 3), got %v", spanVal)
	}
	s0, _ := Force(spanTuple.Elements[0]).(IntValue).ToInt()
	s1, _ := Force(spanTuple.Elements[1]).(IntValue).ToInt()
	if s0 != 0 || s1 != 3 {
		t.Fatalf("expected span (0, 3), got (%d, %d)", s0, s1)
	}
}

func TestRegexNamedGroupsAndPatternMatch(t *testing.T) {
	input := `
r = /(?P<year>\d{4})-(?P<month>\d{2})/
s = "2026-10"
m = s =~ r

year = m["year"]
month = m["month"]
groups = m.Groups()
named = m.NamedGroups()

res_y = match s:
    case /(?P<y>\d{4})-(?P<m>\d{2})/ as res:
        res["y"]
    case _:
        "fail"

is_match = "2026" is /^\d+$/
is_not_match = "abc" is /^\d+$/
`
	env, _ := runEvalCode(t, input)

	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}

	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}

	checkStr("year", "2026")
	checkStr("month", "10")
	checkStr("res_y", "2026")
	checkBool("is_match", true)
	checkBool("is_not_match", false)

	// groups
	gVal, _ := env.Get("groups")
	gTuple, ok := Force(gVal).(*TupleValue)
	if !ok || len(gTuple.Elements) != 2 {
		t.Fatalf("expected 2 capture groups in groups(), got %v", gVal)
	}

	// named
	nVal, _ := env.Get("named")
	nDict, ok := Force(nVal).(*DictValue)
	if !ok || len(nDict.Entries) != 2 {
		t.Fatalf("expected 2 entries in named, got %v", nVal)
	}
}

func TestRegexEdgeCasesAndReviewFixes(t *testing.T) {
	input := `
r = /(?P<year>\d{4})-(?P<month>\d{2})/
s = "2026-10"
m = s =~ r

empty_span = m.Span("")
neg_span = m.Span(-1)
neg_start = m.Start(-1)
neg_end = m.End(-1)

r_slash = /a\/b/
r_str = r_slash.Inspect()
is_escaped = "a/b" is /^a\/b$/

val = 0
match s:
    case /\d{4}/:
        val = 42
    case _:
        val = 99
`
	env, _ := runEvalCode(t, input)

	checkInt := func(varName string, expected int) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		iv, ok := Force(val).(IntValue)
		if !ok {
			t.Fatalf("%s not IntValue: %v", varName, val)
		}
		i64, _ := iv.ToInt()
		if i64 != expected {
			t.Fatalf("%s: expected %d, got %d", varName, expected, i64)
		}
	}

	esVal, _ := env.Get("empty_span")
	esTuple := Force(esVal).(*TupleValue)
	s0, _ := Force(esTuple.Elements[0]).(IntValue).ToInt()
	s1, _ := Force(esTuple.Elements[1]).(IntValue).ToInt()
	if s0 != -1 || s1 != -1 {
		t.Fatalf("expected (-1, -1), got (%d, %d)", s0, s1)
	}

	checkInt("neg_start", 5)
	checkInt("neg_end", 7)

	rStrVal, _ := env.Get("r_str")
	if string(Force(rStrVal).(StringValue)) != "/a\\/b/" {
		t.Fatalf("expected /a\\/b/, got %s", Force(rStrVal))
	}

	isEscVal, _ := env.Get("is_escaped")
	if !bool(Force(isEscVal).(BoolValue)) {
		t.Fatalf("expected is_escaped to be true")
	}

	checkInt("val", 42)
}
