package upstream_test

import (
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/upstream"
)

// AC-009: Within cooldown window → Check returns true (blocked)
func TestAC009_WithinCooldownBlocked(t *testing.T) {
	cd := upstream.NewCooldownTracker(1 * time.Second)

	cd.Record("test-key")

	if !cd.Check("test-key") {
		t.Error("expected Check to return true (blocked) within cooldown window")
	}
}

// AC-010: After cooldown expires → Check returns false (allowed)
func TestAC010_AfterCooldownAllowed(t *testing.T) {
	cd := upstream.NewCooldownTracker(100 * time.Millisecond)

	cd.Record("test-key")

	// Within cooldown
	if !cd.Check("test-key") {
		t.Error("expected blocked within cooldown")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	if cd.Check("test-key") {
		t.Error("expected Check to return false (allowed) after cooldown expires")
	}
}

// AC-011: Record updates timestamp
func TestAC011_RecordUpdatesTimestamp(t *testing.T) {
	cd := upstream.NewCooldownTracker(100 * time.Millisecond)

	cd.Record("test-key")
	time.Sleep(80 * time.Millisecond)

	// Re-record should reset the window
	cd.Record("test-key")

	// Should still be blocked because we just re-recorded
	if !cd.Check("test-key") {
		t.Error("expected blocked after re-recording cooldown")
	}

	// Wait a bit — should still be blocked because re-record reset the timer
	time.Sleep(50 * time.Millisecond)
	if !cd.Check("test-key") {
		t.Error("expected still blocked — re-record reset cooldown timer")
	}
}

// AC-012: Key granularity — different keys have independent cooldowns
func TestAC012_KeyGranularity(t *testing.T) {
	cd := upstream.NewCooldownTracker(1 * time.Second)

	cd.Record("key-a")

	if !cd.Check("key-a") {
		t.Error("expected key-a to be blocked")
	}

	// key-b should not be blocked
	if cd.Check("key-b") {
		t.Error("expected key-b to be allowed — independent of key-a")
	}
}

// AC-012: Unrecorded key → Check returns false (allowed)
func TestAC012_UnrecordedKeyAllowed(t *testing.T) {
	cd := upstream.NewCooldownTracker(1 * time.Second)

	if cd.Check("never-recorded") {
		t.Error("expected unrecorded key to be allowed")
	}
}

// AC-012: Configurable duration
func TestAC012_ConfigurableDuration(t *testing.T) {
	// Short duration
	cd := upstream.NewCooldownTracker(50 * time.Millisecond)
	cd.Record("test-key")

	if !cd.Check("test-key") {
		t.Error("expected blocked within 50ms cooldown")
	}

	time.Sleep(60 * time.Millisecond)

	if cd.Check("test-key") {
		t.Error("expected allowed after 50ms cooldown expires")
	}
}
