package chanxtest_test

import "testing"

// Whole-file examples cannot hold tests, so their bodies run here.
func TestExampleBodies(t *testing.T) {
	t.Run("Connect", testChat)
	t.Run("Spy", testJoinAnnouncement)
}
