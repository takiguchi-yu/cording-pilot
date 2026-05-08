package pkg

import "testing"

func TestReverseString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "empty", in: "", out: ""},
		{name: "ascii", in: "abc", out: "cba"},
		{name: "unicode", in: "あいう", out: "ういあ"},
		{name: "mixed", in: "Go言語", out: "語言oG"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := ReverseString(tc.in); got != tc.out {
				t.Fatalf("ReverseString(%q) = %q, want %q", tc.in, got, tc.out)
			}
		})
	}
}
