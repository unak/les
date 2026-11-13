package eval

import "testing"

// Lazy thunks must observe the bindings visible when they were created, not
// the values that variables are rebound to before the thunk is forced.
func TestLazyThunkCapturesBindingsAcrossRebinding(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "rebound loop variable pushed into tuple",
			code: `j = 0
acc: (Int, ...) = ()
for j < 3:
    acc = acc << j
    j = j + 1
print acc`,
			expected: "(0, 1, 2)",
		},
		{
			name: "computed element pushed into tuple",
			code: `i = 0
acc: (Int, ...) = ()
for i < 4:
    acc = acc << i * i
    i = i + 1
print acc`,
			expected: "(0, 1, 4, 9)",
		},
		{
			name: "tuple literal element",
			code: `j = 0
ts: ((Int, Int), ...) = ()
for j < 3:
    ts = ts << (j, j * 2)
    j = j + 1
print ts`,
			expected: "((0, 0), (1, 2), (2, 4))",
		},
		{
			name: "lazy variable declared before rebinding",
			code: `j = 0
x = j + 10
j = 5
print x`,
			expected: "10",
		},
		{
			name: "function argument",
			code: `def wrap(v: Int) -> (Int,):
    (v,)
j = 0
t = wrap(j)
j = 1
print t`,
			expected: "(0,)",
		},
		{
			name: "lambda created in loop",
			code: `j = 0
fs: (Int -> Int, ...) = ()
for j < 3:
    fs = fs << |x|: x + j
    j = j + 1
print fs[0](100), " ", fs[1](100), " ", fs[2](100)`,
			expected: "100 101 102",
		},
		{
			name: "loop inside function",
			code: `def collect(n: Int) -> (Int, ...):
    i = 0
    acc: (Int, ...) = ()
    for i < n:
        acc = acc << i
        i = i + 1
    acc
print collect(3)`,
			expected: "(0, 1, 2)",
		},
		{
			name: "default parameter referring to rebound parameter",
			code: `def f(a: Int, b: Int = a + 1) -> Int:
    a = a * 10
    b
print f(1)`,
			expected: "2",
		},
		{
			name: "struct value pushed before rebinding",
			code: `struct Counter:
    n: Int = 0
    def Next() -> Counter:
        Counter(n: @n + 1)
c = Counter()
cs: (Counter, ...) = ()
i = 0
for i < 3:
    cs = cs << c
    c = c.Next()
    i = i + 1
print cs[0].n, cs[1].n, cs[2].n`,
			expected: "012",
		},
		{
			name: "element referring to previous version of the tuple",
			code: `acc: (Int, ...) = (1,)
i = 0
for i < 3:
    acc = acc << acc[i] * 2
    i = i + 1
print acc`,
			expected: "(1, 2, 4, 8)",
		},
		{
			name: "captured elements stay lazy",
			code: `j = 0
acc: (Int, ...) = ()
for j < 3:
    acc = acc << 10 / j
    j = j + 1
print acc[1], " ", acc[2]`,
			expected: "10 5",
		},
		{
			name: "forward reference to later definition",
			code: `y = later(2)
def later(n: Int) -> Int:
    n * 100
print y`,
			expected: "200",
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
