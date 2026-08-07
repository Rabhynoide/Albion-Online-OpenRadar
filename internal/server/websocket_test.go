package server

import (
	"math"
	"testing"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"

	"github.com/nospy/albion-openradar/internal/photon"
)

// Locks the JSON wire shape broadcast to the web front-end. The front reads
// params[252]/params[253] as numeric string keys; switching the param map
// type (from int to byte) must not alter this contract.
func TestBroadcastEvent_JSONShape(t *testing.T) {
	event := &photon.EventData{
		Code: 3,
		Parameters: map[byte]interface{}{
			0:   int32(42),
			252: byte(3),
		},
	}
	payload := map[string]interface{}{
		"code":       event.Code,
		"parameters": event.Parameters,
	}
	out, err := json.Marshal(payload)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, `"code":3`)
	require.Contains(t, s, `"252":3`)
	require.Contains(t, s, `"0":42`)
}

func TestBroadcastEvent_ByteArray_BufferShape(t *testing.T) {
	event := &photon.EventData{
		Code: 3,
		Parameters: map[byte]interface{}{
			1: photon.ByteArray{0x01, 0x02, 0xff},
		},
	}
	out, err := json.Marshal(event.Parameters)
	require.NoError(t, err)
	require.Contains(t, string(out), `{"type":"Buffer","data":[1,2,255]}`)
}

// @verified: a single NaN-carrying message (a real, observed occurrence - a raw Photon float32
// parameter whose bit pattern happens to decode to NaN) used to fail json.Marshal for the WHOLE
// batch, dropping every message in it, not just the offending one.
func TestMarshalBatch_DropsOnlyUnmarshalableMessages(t *testing.T) {
	ws := &WebSocketHandler{}
	batch := []interface{}{
		map[string]interface{}{"code": "event", "id": 1},
		map[string]interface{}{"code": "event", "id": 2, "bad": math.NaN()},
		map[string]interface{}{"code": "event", "id": 3},
	}

	data, kept := ws.marshalBatch(batch)
	require.Equal(t, 2, kept)
	require.NotEmpty(t, data)

	var decoded WSBatchMessage
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Messages, 2)
}

func TestMarshalBatch_AllUnmarshalableDropsWholeBatch(t *testing.T) {
	ws := &WebSocketHandler{}
	batch := []interface{}{
		map[string]interface{}{"bad": math.NaN()},
		map[string]interface{}{"bad": math.Inf(1)},
	}

	data, kept := ws.marshalBatch(batch)
	require.Equal(t, 0, kept)
	require.Nil(t, data)
}

func TestMarshalBatch_HappyPathMarshalsWholeBatchInOneShot(t *testing.T) {
	ws := &WebSocketHandler{}
	batch := []interface{}{
		map[string]interface{}{"id": 1},
		map[string]interface{}{"id": 2},
	}

	data, kept := ws.marshalBatch(batch)
	require.Equal(t, 2, kept)

	var decoded WSBatchMessage
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Messages, 2)
}
