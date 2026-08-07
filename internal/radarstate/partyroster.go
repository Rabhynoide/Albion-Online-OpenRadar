package radarstate

import (
	"encoding/hex"
	"sync"

	"github.com/nospy/albion-openradar/internal/photon"
)

// PartyRoster is a Go port of web/scripts/data/PartyRoster.js: tracks the local player's
// current party membership by display name, learned live from Photon Party events (event 231
// PartyJoined, 232 PartyDisbanded, 235 PartyPlayerLeft - wire layout confirmed 2026-08-01 via a
// real capture, see docs/technical/PROTOCOL18_PARAM_LAYOUTS.md's "Party events" section).
// Session-only by design: a party roster shouldn't survive past the party itself.
type PartyRoster struct {
	mu         sync.RWMutex
	members    map[string]struct{}
	guidToName map[string]string // last-seen GUID(hex) -> name, for PartyPlayerLeft lookups
}

func NewPartyRoster() *PartyRoster {
	return &PartyRoster{
		members:    make(map[string]struct{}),
		guidToName: make(map[string]string),
	}
}

// HandlePartyJoined ports handlePartyJoined: params[9] is the member name array, params[8] the
// concatenated 16-byte member GUIDs in the same order, zipped into a GUID->name map so
// HandlePartyPlayerLeft (which only gets a GUID, no name) can resolve who left.
func (r *PartyRoster) HandlePartyJoined(p Params) {
	names := paramStringSlice(p, 9)
	if names == nil {
		return
	}
	guidBuffer, _ := p[8].(photon.ByteArray)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.members = make(map[string]struct{}, len(names))
	r.guidToName = make(map[string]string, len(names))
	for _, name := range names {
		r.members[name] = struct{}{}
	}
	if len(guidBuffer) == len(names)*16 {
		for i, name := range names {
			guid := hex.EncodeToString(guidBuffer[i*16 : i*16+16])
			r.guidToName[guid] = name
		}
	}
}

// HandlePartyPlayerLeft ports handlePartyPlayerLeft: params[1] carries only the leaving
// member's GUID, not their name - resolved via the map built by HandlePartyJoined.
func (r *PartyRoster) HandlePartyPlayerLeft(p Params) {
	guidBuffer, ok := p[1].(photon.ByteArray)
	if !ok || len(guidBuffer) == 0 {
		return
	}
	guid := hex.EncodeToString(guidBuffer)

	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.guidToName[guid]; ok {
		delete(r.members, name)
		delete(r.guidToName, guid)
	}
}

// HandlePartyDisbanded ports handlePartyDisbanded.
func (r *PartyRoster) HandlePartyDisbanded() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members = make(map[string]struct{})
	r.guidToName = make(map[string]string)
}

// IsPartyMember reports whether nickname is currently a tracked party member.
func (r *PartyRoster) IsPartyMember(nickname string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.members[nickname]
	return ok
}

// Reset mirrors PartyRoster.js's reset(), called on radar session teardown: a stale roster
// from a party that changed or disbanded while the radar wasn't running would otherwise
// wrongly keep excluding people on the next session.
func (r *PartyRoster) Reset() {
	r.HandlePartyDisbanded()
}
