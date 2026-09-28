package tools

import "testing"

func TestFindCircuit(t *testing.T) {
	for q, want := range map[string]string{"spa": "spa", "Monaco": "monaco", "Silverstone": "silverstone", "Suzuka": "suzuka", "Baku": "baku"} {
		got, err := findCircuit(q)
		if err != nil || got != want {
			t.Errorf("findCircuit(%q) = %q, %v; want %q", q, got, err, want)
		}
	}
	// Italy has Monza and Imola; the moon has neither.
	for _, q := range []string{"Italy", "moon"} {
		if _, err := findCircuit(q); err == nil {
			t.Errorf("findCircuit(%q): want an error", q)
		}
	}
}
