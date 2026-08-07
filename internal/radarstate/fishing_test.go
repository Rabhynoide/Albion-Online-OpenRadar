package radarstate

import "testing"

func TestFishingState_SpawnAndUpdate(t *testing.T) {
	s := NewFishingState()
	s.HandleNewFishingZoneObject(Params{0: int32(1), 1: []float32{10, 20}, 2: int32(3), 3: int32(7), 4: "FISH_FRESHWATER"})

	fish := s.Snapshot()[0]
	if fish.TotalSize != 10 {
		t.Errorf("TotalSize = %d, want 10 (sizeSpawned+sizeLeftToSpawn)", fish.TotalSize)
	}

	// Unlike most handlers, a re-observation updates size in place (a fish being caught).
	s.HandleNewFishingZoneObject(Params{0: int32(1), 1: []float32{10, 20}, 2: int32(1), 3: int32(7), 4: "FISH_FRESHWATER"})
	if got := s.Snapshot()[0].SizeSpawned; got != 1 {
		t.Errorf("SizeSpawned after update = %d, want 1 (re-observation should overwrite, not just touch)", got)
	}
}

func TestFishingState_MissingTypeIgnored(t *testing.T) {
	s := NewFishingState()
	s.HandleNewFishingZoneObject(Params{0: int32(1), 1: []float32{10, 20}})

	if s.Len() != 0 {
		t.Error("an event with no fish type should be ignored")
	}
}

func TestFishingState_End(t *testing.T) {
	s := NewFishingState()
	s.HandleNewFishingZoneObject(Params{0: int32(1), 1: []float32{0, 0}, 4: "FISH"})
	s.HandleFishingFinished(Params{0: int32(1)})

	if s.Len() != 0 {
		t.Error("Len() should be 0 after HandleFishingFinished")
	}
}
