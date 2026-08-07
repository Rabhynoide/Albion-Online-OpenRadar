package overlay

import "testing"

func TestPosTracker_SyncCreatesAndInterpolates(t *testing.T) {
	tr := newPosTracker()
	tr.sync([]idPos{{id: 1, x: 10, y: 20}}, 0, 0, 1) // t=1 -> snaps to target

	pos := tr.get(1)
	if pos == nil {
		t.Fatal("expected id 1 to be tracked")
	}
	if pos.HX != -10 || pos.HY != 20 { // interpolate: hX=-1*posX+lpX, hY=posY-lpY
		t.Errorf("HX=%v HY=%v, want HX=-10 HY=20", pos.HX, pos.HY)
	}
}

func TestPosTracker_GetMissingReturnsNil(t *testing.T) {
	tr := newPosTracker()
	if tr.get(999) != nil {
		t.Error("get() on an untracked id should return nil")
	}
}

func TestPosTracker_SyncPrunesMissingIDs(t *testing.T) {
	tr := newPosTracker()
	tr.sync([]idPos{{id: 1, x: 0, y: 0}, {id: 2, x: 0, y: 0}}, 0, 0, 1)

	tr.sync([]idPos{{id: 1, x: 0, y: 0}}, 0, 0, 1) // id 2 no longer present

	if tr.get(1) == nil {
		t.Error("id 1 should still be tracked")
	}
	if tr.get(2) != nil {
		t.Error("id 2 should have been pruned")
	}
}

func TestPosTracker_SyncPreservesExistingPositionAcrossCalls(t *testing.T) {
	tr := newPosTracker()
	tr.sync([]idPos{{id: 1, x: 100, y: 100}}, 0, 0, 1) // snaps to target
	first := *tr.get(1)

	// A second sync with a small lerpT should move gradually, not re-create/snap again.
	tr.sync([]idPos{{id: 1, x: 100, y: 100}}, 50, 50, 0.5)

	second := tr.get(1)
	if *second == first {
		t.Error("a subsequent sync should lerp toward the (now different, lpX/lpY changed) target, not stay frozen")
	}
}
