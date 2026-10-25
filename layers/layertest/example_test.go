package layertest_test

import (
	"testing"

	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/inmemory"
	"github.com/huynguyengl99/chanx-go/layers/layertest"
)

// testLayer is how a backend proves itself: name it TestLayer in your package and
// pass a factory returning a fresh layer.
func testLayer(t *testing.T) {
	layertest.Run(t, func(t *testing.T) layers.Layer { return inmemory.New() })
}

func ExampleRun() {
	_ = testLayer
}
