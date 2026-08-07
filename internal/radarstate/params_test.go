package radarstate

import (
	"testing"

	"github.com/nospy/albion-openradar/internal/photon"
)

func TestParamInt_AcceptsEveryNumericType(t *testing.T) {
	p := Params{
		1: byte(1), 2: int16(2), 3: int32(3), 4: int64(4), 5: float32(5), 6: float64(6),
	}
	for key := byte(1); key <= 6; key++ {
		got, ok := paramInt(p, key)
		if !ok || got != int(key) {
			t.Errorf("paramInt(key=%d) = (%d, %v), want (%d, true)", key, got, ok, key)
		}
	}
}

func TestParamInt_MissingOrWrongType(t *testing.T) {
	p := Params{1: "not a number"}
	if _, ok := paramInt(p, 1); ok {
		t.Error("paramInt should report false for a wrong-typed value")
	}
	if _, ok := paramInt(p, 99); ok {
		t.Error("paramInt should report false for a missing key")
	}
}

func TestParamIntDefault(t *testing.T) {
	p := Params{1: int32(7)}
	if got := paramIntDefault(p, 1, 99); got != 7 {
		t.Errorf("paramIntDefault(present) = %d, want 7", got)
	}
	if got := paramIntDefault(p, 2, 99); got != 99 {
		t.Errorf("paramIntDefault(missing) = %d, want 99 (default)", got)
	}
}

func TestParamString(t *testing.T) {
	p := Params{1: "hello"}
	got, ok := paramString(p, 1)
	if !ok || got != "hello" {
		t.Errorf("paramString = (%q, %v), want (hello, true)", got, ok)
	}
	if got := paramStringDefault(p, 2, "fallback"); got != "fallback" {
		t.Errorf("paramStringDefault(missing) = %q, want fallback", got)
	}
}

func TestParamBool(t *testing.T) {
	p := Params{1: true}
	got, ok := paramBool(p, 1)
	if !ok || !got {
		t.Errorf("paramBool = (%v, %v), want (true, true)", got, ok)
	}
}

func TestParamPosition(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
	}{
		{"float32 slice", []float32{1.5, 2.5}},
		{"float64 slice", []float64{1.5, 2.5}},
		{"interface slice", []interface{}{float32(1.5), float32(2.5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Params{1: tt.v}
			x, y, ok := paramPosition(p, 1)
			if !ok || x != 1.5 || y != 2.5 {
				t.Errorf("paramPosition(%s) = (%v, %v, %v), want (1.5, 2.5, true)", tt.name, x, y, ok)
			}
		})
	}
}

func TestParamPosition_TooShortOrMissing(t *testing.T) {
	p := Params{1: []float32{1.5}}
	if _, _, ok := paramPosition(p, 1); ok {
		t.Error("paramPosition should report false for a 1-element array")
	}
	if _, _, ok := paramPosition(p, 99); ok {
		t.Error("paramPosition should report false for a missing key")
	}
}

func TestParamIntSlice(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want []int
	}{
		{"int32 slice", []int32{1, 2, 3}, []int{1, 2, 3}},
		{"ByteArray", photon.ByteArray{1, 2, 3}, []int{1, 2, 3}},
		{"interface slice", []interface{}{int32(1), float64(2), byte(3)}, []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Params{1: tt.v}
			got := paramIntSlice(p, 1)
			if len(got) != len(tt.want) {
				t.Fatalf("paramIntSlice(%s) = %v, want %v", tt.name, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("paramIntSlice(%s)[%d] = %d, want %d", tt.name, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParamFloat32Slice(t *testing.T) {
	p := Params{1: []float32{1, 2, 3, 4}}
	got := paramFloat32Slice(p, 1)
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("paramFloat32Slice = %v", got)
	}
}

func TestParamStringSlice(t *testing.T) {
	p := Params{1: []string{"a", "b"}}
	got := paramStringSlice(p, 1)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("paramStringSlice = %v", got)
	}
}
