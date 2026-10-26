package layers_test

import (
	"encoding/json"
	"fmt"

	"go.chanx.dev/layers"
)

// Int reads an integer whatever the serializer made of it: msgpack's uint8, a
// json.Number, a float64 without a fraction.
func ExampleInt() {
	for _, v := range []any{uint8(7), json.Number("7"), 7.0} {
		n, ok := layers.Int(v)
		fmt.Println(n, ok)
	}
	_, ok := layers.Int(7.5)
	fmt.Println(ok)
	// Output:
	// 7 true
	// 7 true
	// 7 true
	// false
}
