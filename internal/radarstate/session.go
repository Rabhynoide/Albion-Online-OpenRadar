package radarstate

import (
	"encoding/binary"
	"math"

	"github.com/nospy/albion-openradar/internal/photon"
)

// Session tracks the local player's own current zone and position - a Go port of the subset
// of web/scripts/core/EventRouter.js's module-level state (lpX/lpY, window.currentMapId) that
// radarstate's handlers/ZoneGraph need to operate (city/pvp-type resolution, GPS road
// discovery). The fuller Mist-portal-chain PvP-inheritance logic EventRouter.js also owns
// (tracking which sanctuary a Mist was entered from across multiple hops) is not ported here -
// a deliberate first-pass simplification, not an oversight: see
// docs/technical/NATIVE_OVERLAY_CLIENT.md for the follow-up.
type Session struct {
	CurrentZoneID string
	LocalX        float32
	LocalY        float32
}

func NewSession() *Session {
	return &Session{}
}

// SetLocalPosition mirrors EventRouter.js's updateLocalPlayerPosition.
func (s *Session) SetLocalPosition(x, y float32) {
	s.LocalX, s.LocalY = x, y
}

// HandleMoveRequest ports the local-player-position half of onRequest's Move handling
// (opcode 21 legacy / Move): position=[1], either a direct 2-element array or a raw byte
// buffer (two little-endian float32s).
func (s *Session) HandleMoveRequest(p Params) {
	if x, y, ok := paramPosition(p, 1); ok {
		s.SetLocalPosition(x, y)
		return
	}
	if raw, ok := p[1].(photon.ByteArray); ok && len(raw) >= 8 {
		x := math.Float32frombits(binary.LittleEndian.Uint32(raw[0:4]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(raw[4:8]))
		s.SetLocalPosition(x, y)
	}
}

// HandleJoinPosition ports decodeJoinPosition (the Join response's own position field,
// param index depends on the caller - see EventRouter.js's handleChangeClusterResponse).
func (s *Session) HandleJoinPosition(raw interface{}) {
	switch v := raw.(type) {
	case photon.ByteArray:
		if len(v) >= 8 {
			x := math.Float32frombits(binary.LittleEndian.Uint32(v[0:4]))
			y := math.Float32frombits(binary.LittleEndian.Uint32(v[4:8]))
			s.SetLocalPosition(x, y)
		}
	case []float32:
		if len(v) >= 2 {
			s.SetLocalPosition(v[0], v[1])
		}
	case []interface{}:
		if len(v) >= 2 {
			x, xok := toFloat32(v[0])
			y, yok := toFloat32(v[1])
			if xok && yok {
				s.SetLocalPosition(x, y)
			}
		}
	}
}

// ChangeZone mirrors the zone-transition half of applyMapChange: updates CurrentZoneID and, if
// both zones are known, reports the transition to graph for GPS road discovery (a novel
// transition graph can't already explain is exactly the "must be an Avalon Road" signal).
func (s *Session) ChangeZone(newZoneID string, graph *ZoneGraph) {
	previous := s.CurrentZoneID
	s.CurrentZoneID = newZoneID
	if graph == nil || previous == "" || newZoneID == "" || previous == newZoneID {
		return
	}
	var pos *[2]float64
	if s.LocalX != 0 || s.LocalY != 0 {
		pos = &[2]float64{float64(s.LocalX), float64(s.LocalY)}
	}
	graph.ReportTransition(previous, newZoneID, pos)
}
