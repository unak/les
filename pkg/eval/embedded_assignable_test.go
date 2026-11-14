package eval

import "testing"

// Passing an outer struct where an embedded struct is expected does not
// convert the value: it stays an instance of the outer struct, and member
// access finds the outer struct's members first.
func TestEmbeddedStructValueIsNotConverted(t *testing.T) {
	const defs = `struct Base:
    k: Int
    def Name() -> String:
        "base"
struct Derived:
    Base
    def Name() -> String:
        "derived"
struct Deep:
    Derived
d = Derived(Base: Base(k: 1))
`
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "is on the embedded struct, also transitively",
			code:     defs + `print (d is Base), " ", (Deep(Derived: d) is Base), " ", (Base(k: 1) is Derived)`,
			expected: "true true false",
		},
		{
			name: "unannotated parameter",
			code: defs + `def greet(b):
    b.Name()
print greet(d), " ", greet(Base(k: 1)), " ", greet(Deep(Derived: d))`,
			expected: "derived base derived",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runLesCode(tt.code)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if out != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, out)
			}
		})
	}
}
