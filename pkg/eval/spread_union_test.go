package eval

import "testing"

func TestSpreadUnionOfTuplesAtRuntime(t *testing.T) {
	code := `
def nats(n: Int):
    (n, nats(n + 1))
def take(k: Int, s):
    k <= 0 ? () : (s[0], ...take(k - 1, s[1]))
def pick(b: Bool) -> (Int, ...) | (String, ...):
    b ? (1,) : ("x",)
print take(3, nats(0)), " ", (...pick(true), 2), " ", (...pick(false), 2)
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "(0, 1, 2) (1, 2) (x, 2)"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}
