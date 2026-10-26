// Package stream helps send streamed output, such as an LLM response, over a
// WebSocket.
package stream

import "strings"

// DeltaTracker converts cumulative text snapshots (what LLM and generator APIs
// usually yield) into append-only deltas, so a WebSocket does not re-send the
// whole response on every tick.
//
//	var d stream.DeltaTracker
//	d.Push("Hel")   // "Hel"
//	d.Push("Hello") // "lo"
//
// When a snapshot does not extend the previous one (a provider rewrote the
// tail), the full new text is returned; Advance also reports it.
type DeltaTracker struct {
	Text string
}

// Push returns the new text in snapshot relative to what was already emitted.
func (d *DeltaTracker) Push(snapshot string) string {
	delta, _ := d.Advance(snapshot)
	return delta
}

// Advance returns the delta and whether the snapshot diverged, in which case the
// delta is a full replacement rather than an append.
func (d *DeltaTracker) Advance(snapshot string) (delta string, rewritten bool) {
	if strings.HasPrefix(snapshot, d.Text) {
		delta = snapshot[len(d.Text):]
		d.Text = snapshot
		return delta, false
	}
	d.Text = snapshot
	return snapshot, true
}

// Reset forgets the accumulated text.
func (d *DeltaTracker) Reset() { d.Text = "" }
