package layering

import (
	"context"
	"testing"

	"go.chanx.dev/layers/v2/inmemory"
)

func TestFanOut(t *testing.T) {
	msg, err := FanOut(context.Background(), inmemory.New())
	if err != nil || msg["key"] != "user:42" {
		t.Fatal(msg, err)
	}
}

func TestAliased(t *testing.T) {
	e := Aliased()
	if err := NotifyJobDone(context.Background(), e.Layers(), "ann"); err != nil {
		t.Fatal(err)
	}
}
