package chanx_test

import (
	"testing"

	"go.uber.org/goleak"
)

// Every connection runs a reader, a writer and a layer goroutine; a missed
// cleanup path shows up here.
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
