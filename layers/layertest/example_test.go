package layertest_test

import (
	"testing"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	"go.chanx.dev/layers/v2/layertest"
)

// testLayer is how a backend proves itself: name it TestLayer in your package and
// pass a factory returning a fresh layer.
func testLayer(t *testing.T) {
	layertest.Run(t, func(t *testing.T) layers.Layer { return inmemory.New() })
}

func ExampleRun() {
	_ = testLayer
}
