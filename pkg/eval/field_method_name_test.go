package eval

import "testing"

func TestStructFunctionFieldWithoutSameNameMethod(t *testing.T) {
	out, err := runLesCode(`
struct Shape:
    area: () -> Int
    def twice() -> Int:
        @area() * 2
s = Shape(area: ||: 2)
print s.area(), " ", s.twice()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "2 4" {
		t.Fatalf("expected %q, got %q", "2 4", out)
	}
}

func TestStructFieldAndMethodDifferentCase(t *testing.T) {
	out, err := runLesCode(`
struct P:
    size: Int
    def Size() -> Int:
        @size + 100
p = P(size: 1)
print p.size, " ", p.Size()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "1 101" {
		t.Fatalf("expected %q, got %q", "1 101", out)
	}
}

func TestStructOuterMembersShadowPromoted(t *testing.T) {
	out, err := runLesCode(`
struct Inner:
    v: Int
    def hello() -> String:
        "inner"
struct Outer:
    Inner
    v: String
    def hello() -> String:
        "outer"
o = Outer(v: "s")
print o.v, " ", o.hello()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "s outer" {
		t.Fatalf("expected %q, got %q", "s outer", out)
	}
}
