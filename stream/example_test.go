package stream_test

import (
	"fmt"

	"github.com/huynguyengl99/chanx-go/stream"
)

// An LLM API yields the whole text so far on every tick; send only what is new.
func ExampleDeltaTracker() {
	var d stream.DeltaTracker
	for _, snapshot := range []string{"Hel", "Hello", "Hello, wor", "Hello, world"} {
		fmt.Printf("%q\n", d.Push(snapshot))
	}
	// Output:
	// "Hel"
	// "lo"
	// ", wor"
	// "ld"
}
