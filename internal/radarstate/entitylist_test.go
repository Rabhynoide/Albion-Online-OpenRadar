package radarstate

import (
	"testing"
	"time"
)

func TestEntityList_UpsertDoesNotOverwriteExisting(t *testing.T) {
	l := newEntityList[int, string]()
	calls := 0
	l.upsert(1, func() string { calls++; return "first" })
	l.upsert(1, func() string { calls++; return "second" })

	v, ok := l.get(1)
	if !ok || v != "first" {
		t.Errorf("get(1) = (%q, %v), want (first, true)", v, ok)
	}
	if calls != 1 {
		t.Errorf("create func called %d times, want 1 (touch must not re-create)", calls)
	}
}

func TestEntityList_Replace(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "a")
	l.replace(1, "b")

	v, ok := l.get(1)
	if !ok || v != "b" {
		t.Errorf("get(1) = (%q, %v), want (b, true)", v, ok)
	}
}

func TestEntityList_Remove(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "a")
	l.remove(1)

	if _, ok := l.get(1); ok {
		t.Error("get(1) should not find a removed entity")
	}
}

func TestEntityList_Clear(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "a")
	l.replace(2, "b")
	l.clear()

	if l.len() != 0 {
		t.Errorf("len() = %d, want 0 after clear", l.len())
	}
}

func TestEntityList_Snapshot(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "a")
	l.replace(2, "b")

	snap := l.snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
}

func TestEntityList_CleanupStaleRemovesOldEntries(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "old")
	l.lastSeen[1] = time.Now().Add(-time.Hour) // simulate an entry seen an hour ago
	l.replace(2, "fresh")

	removed := l.cleanupStale(time.Minute)

	if removed != 1 {
		t.Errorf("cleanupStale removed %d, want 1", removed)
	}
	if _, ok := l.get(1); ok {
		t.Error("stale entry should have been removed")
	}
	if _, ok := l.get(2); !ok {
		t.Error("fresh entry should remain")
	}
}

func TestEntityList_Len(t *testing.T) {
	l := newEntityList[int, string]()
	l.replace(1, "a")
	l.replace(2, "b")
	l.replace(3, "c")

	if l.len() != 3 {
		t.Errorf("len() = %d, want 3", l.len())
	}
}
