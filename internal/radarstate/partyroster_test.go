package radarstate

import (
	"testing"

	"github.com/nospy/albion-openradar/internal/photon"
)

func guidBytes(b byte) photon.ByteArray {
	g := make(photon.ByteArray, 16)
	for i := range g {
		g[i] = b
	}
	return g
}

func TestPartyRoster_JoinedTracksMembers(t *testing.T) {
	r := NewPartyRoster()
	guids := append(append(photon.ByteArray{}, guidBytes(0xAA)...), guidBytes(0xBB)...)

	r.HandlePartyJoined(Params{
		8: guids,
		9: []string{"Alice", "Bob"},
	})

	if !r.IsPartyMember("Alice") || !r.IsPartyMember("Bob") {
		t.Error("both Alice and Bob should be tracked as party members")
	}
	if r.IsPartyMember("Charlie") {
		t.Error("Charlie was never in the roster")
	}
}

func TestPartyRoster_PlayerLeftResolvesNameByGUID(t *testing.T) {
	r := NewPartyRoster()
	guids := append(append(photon.ByteArray{}, guidBytes(0xAA)...), guidBytes(0xBB)...)
	r.HandlePartyJoined(Params{8: guids, 9: []string{"Alice", "Bob"}})

	r.HandlePartyPlayerLeft(Params{1: guidBytes(0xAA)})

	if r.IsPartyMember("Alice") {
		t.Error("Alice should have been removed after PartyPlayerLeft")
	}
	if !r.IsPartyMember("Bob") {
		t.Error("Bob should remain a member")
	}
}

func TestPartyRoster_Disbanded(t *testing.T) {
	r := NewPartyRoster()
	guids := append(append(photon.ByteArray{}, guidBytes(0xAA)...), guidBytes(0xBB)...)
	r.HandlePartyJoined(Params{8: guids, 9: []string{"Alice", "Bob"}})

	r.HandlePartyDisbanded()

	if r.IsPartyMember("Alice") || r.IsPartyMember("Bob") {
		t.Error("no one should be a party member after PartyDisbanded")
	}
}

func TestPartyRoster_MismatchedGUIDLengthSkipsGUIDMap(t *testing.T) {
	r := NewPartyRoster()
	// Only one 16-byte GUID for two names - length mismatch, GUID map should stay empty.
	r.HandlePartyJoined(Params{8: guidBytes(0xAA), 9: []string{"Alice", "Bob"}})

	// Members should still be tracked from the names array...
	if !r.IsPartyMember("Alice") || !r.IsPartyMember("Bob") {
		t.Error("members should still be set from names even with a GUID length mismatch")
	}
	// ...but PartyPlayerLeft can't resolve anyone since the GUID map was never populated.
	r.HandlePartyPlayerLeft(Params{1: guidBytes(0xAA)})
	if !r.IsPartyMember("Alice") {
		t.Error("PartyPlayerLeft should be a no-op when the GUID map is empty")
	}
}

func TestPartyRoster_Reset(t *testing.T) {
	r := NewPartyRoster()
	guids := guidBytes(0xAA)
	r.HandlePartyJoined(Params{8: guids, 9: []string{"Alice"}})

	r.Reset()

	if r.IsPartyMember("Alice") {
		t.Error("Reset should clear the roster")
	}
}
