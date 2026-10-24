package stream

import "testing"

func TestDeltaTracker(t *testing.T) {
	var d DeltaTracker
	for _, c := range []struct {
		in, want  string
		rewritten bool
	}{{"Hel", "Hel", false}, {"Hello", "lo", false}, {"Hello", "", false}, {"Help", "Help", true}, {"Help!", "!", false}} {
		got, rw := d.Advance(c.in)
		if got != c.want || rw != c.rewritten {
			t.Errorf("Advance(%q) = %q, %v", c.in, got, rw)
		}
	}
	d.Reset()
	if d.Push("x") != "x" {
		t.Fatal("reset")
	}
}
