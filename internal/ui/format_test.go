package ui

import "testing"

func TestCompact(t *testing.T) {
	tests := map[int64]string{
		0:          "0",
		82:         "82",
		1_000:      "1k",
		9_540:      "9.5k",
		9_990:      "10k",
		148_841:    "148k",
		4_874_452:  "4.9M",
		52_000_000: "52M",
	}
	for n, want := range tests {
		if got := Compact(n); got != want {
			t.Errorf("Compact(%d) = %q, want %q", n, got, want)
		}
	}
}
