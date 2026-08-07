// Package radarstate is a Go port of web/scripts/handlers/*.js: it interprets already-decoded
// Photon events/requests/responses (internal/photon) into in-memory entity lists, for the
// native overlay client (internal/overlay) - the same interpretation the browser's
// EventRouter.js does today, just running in-process instead of over a WebSocket. Every
// handler here is a direct, deliberately mechanical port of its JS counterpart: the parameter
// indices and types were already capture-verified when the JS was written (see
// docs/technical/PROTOCOL18_PARAM_LAYOUTS.md), so this package translates known-correct logic
// rather than re-deriving wire format from scratch.
package radarstate

import "github.com/nospy/albion-openradar/internal/photon"

// Params is shorthand for a decoded event/request/response's Parameters map (numeric key ->
// dynamically-typed value, exactly as internal/photon's deserializer produces it).
type Params map[byte]interface{}

// paramInt reads an integer parameter, accepting every concrete numeric type
// internal/photon's deserializer can produce for a Photon "int-ish" wire value (byte, int16,
// int32, int64, float32, float64 - Photon's compressed-int encoding can pick any of these
// depending on magnitude). JS has no equivalent step: every JS number is a float64, so
// EventRouter.js/the handlers never have to distinguish these.
func paramInt(p Params, key byte) (int, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case byte:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// paramIntDefault is paramInt with a fallback for a missing/wrong-typed key (mirrors JS's
// `Parameters[k] ?? default` idiom used throughout the handlers).
func paramIntDefault(p Params, key byte, def int) int {
	if v, ok := paramInt(p, key); ok {
		return v
	}
	return def
}

// paramFloat32 reads a float-ish parameter (health values and some position components come
// back as float32, but Photon's compressed-int encoding can also hand back a smaller integer
// type when the value happens to be a whole number).
func paramFloat32(p Params, key byte) (float32, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return 0, false
	}
	return toFloat32(v)
}

// paramString reads a string parameter.
func paramString(p Params, key byte) (string, bool) {
	v, ok := p[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// paramStringDefault is paramString with a fallback.
func paramStringDefault(p Params, key byte, def string) string {
	if s, ok := paramString(p, key); ok {
		return s
	}
	return def
}

// paramBool reads a boolean parameter.
func paramBool(p Params, key byte) (value, ok bool) {
	v, present := p[key]
	if !present || v == nil {
		return false, false
	}
	value, ok = v.(bool)
	return value, ok
}

// paramPosition reads a 2-element position array parameter (Photon typically encodes these as
// []float32; a few events use []float64 or a mixed/whole-number encoding), mirroring the many
// `const position = Parameters[k]; posX = position[0]; posY = position[1]` call sites across
// the JS handlers.
func paramPosition(p Params, key byte) (x, y float32, ok bool) {
	v, present := p[key]
	if !present || v == nil {
		return 0, 0, false
	}
	switch arr := v.(type) {
	case []float32:
		if len(arr) < 2 {
			return 0, 0, false
		}
		return arr[0], arr[1], true
	case []float64:
		if len(arr) < 2 {
			return 0, 0, false
		}
		return float32(arr[0]), float32(arr[1]), true
	case []interface{}:
		if len(arr) < 2 {
			return 0, 0, false
		}
		x, xok := toFloat32(arr[0])
		y, yok := toFloat32(arr[1])
		return x, y, xok && yok
	default:
		return 0, 0, false
	}
}

func toFloat32(v interface{}) (float32, bool) {
	switch n := v.(type) {
	case float32:
		return n, true
	case float64:
		return float32(n), true
	case byte:
		return float32(n), true
	case int16:
		return float32(n), true
	case int32:
		return float32(n), true
	case int64:
		return float32(n), true
	default:
		return 0, false
	}
}

// unwrapData handles a Photon encoding quirk seen on some batch-spawn events (Event 38's
// array parameters): the payload is sometimes a Hashtable wrapping the real array under a
// "data" key instead of the array directly, mirroring the JS handlers'
// `Parameters[k]["data"] ?? Parameters[k]` idiom. Returns v unchanged if it isn't such a
// wrapper.
func unwrapData(v interface{}) interface{} {
	ht, ok := v.(photon.Hashtable)
	if !ok {
		return v
	}
	if data, ok := ht["data"]; ok {
		return data
	}
	return v
}

// paramIntSlice reads an integer-array parameter (ids/enchants/ticks parallel arrays, e.g.
// LocalTreasuresUpdate's Parameters[4]/[6]/[7]), accepting every concrete slice type the
// deserializer can produce.
func paramIntSlice(p Params, key byte) []int {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch arr := unwrapData(v).(type) {
	case []int16:
		out := make([]int, len(arr))
		for i, n := range arr {
			out[i] = int(n)
		}
		return out
	case []int32:
		out := make([]int, len(arr))
		for i, n := range arr {
			out[i] = int(n)
		}
		return out
	case []int64:
		out := make([]int, len(arr))
		for i, n := range arr {
			out[i] = int(n)
		}
		return out
	case photon.ByteArray:
		out := make([]int, len(arr))
		for i, n := range arr {
			out[i] = int(n)
		}
		return out
	case []interface{}:
		out := make([]int, 0, len(arr))
		for _, e := range arr {
			n, ok := paramIntFromAny(e)
			if !ok {
				return nil
			}
			out = append(out, n)
		}
		return out
	default:
		return nil
	}
}

func paramIntFromAny(v interface{}) (int, bool) {
	switch n := v.(type) {
	case byte:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// paramFloat32Slice reads a flat position-pairs array (e.g. LocalTreasuresUpdate's
// Parameters[5], laid out [x0,y0,x1,y1,...]).
func paramFloat32Slice(p Params, key byte) []float32 {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch arr := unwrapData(v).(type) {
	case []float32:
		return arr
	case []float64:
		out := make([]float32, len(arr))
		for i, n := range arr {
			out[i] = float32(n)
		}
		return out
	case []interface{}:
		out := make([]float32, 0, len(arr))
		for _, e := range arr {
			n, ok := toFloat32(e)
			if !ok {
				return nil
			}
			out = append(out, n)
		}
		return out
	default:
		return nil
	}
}

// paramStringSlice reads a string-array parameter.
func paramStringSlice(p Params, key byte) []string {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []string:
		return arr
	case []interface{}:
		out := make([]string, len(arr))
		for i, e := range arr {
			s, _ := e.(string)
			out[i] = s
		}
		return out
	default:
		return nil
	}
}
