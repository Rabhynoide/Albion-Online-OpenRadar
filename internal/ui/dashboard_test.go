package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCaptureStateMsgUpdatesFields(t *testing.T) {
	d := NewDashboard("v0", 5001, true, nil, nil)
	msg := CaptureStateMsg{
		Active: []CaptureSummary{
			{Description: "Wi-Fi", Address: "192.168.1.42", Category: "wifi"},
			{Description: "Realtek", Address: "192.168.1.10", Category: "ethernet"},
		},
		LanAddresses: []string{"192.168.1.42", "192.168.1.10"},
		Status:       "running",
	}
	updated, _ := d.Update(msg)
	out, ok := updated.(Dashboard)
	if !ok {
		t.Fatal("Update did not return Dashboard")
	}
	if len(out.captureInterfaces) != 2 {
		t.Errorf("captureInterfaces len=%d, want 2", len(out.captureInterfaces))
	}
	if out.captureStatus != "running" {
		t.Errorf("status=%q, want running", out.captureStatus)
	}
	if out.lanServerURL == "" {
		t.Error("lanServerURL not derived from first LAN address")
	}
}

func TestUpdateAvailableMsgSetsFields(t *testing.T) {
	d := NewDashboard("1.0.2", 5001, true, nil, nil)
	updated, _ := d.Update(UpdateAvailableMsg{Version: "1.1.0"})
	out, ok := updated.(Dashboard)
	if !ok {
		t.Fatal("Update did not return Dashboard")
	}
	if !out.updateAvailable {
		t.Error("updateAvailable = false, want true")
	}
	if out.latestVersion != "1.1.0" {
		t.Errorf("latestVersion = %q, want 1.1.0", out.latestVersion)
	}
}

// The update notice is appended inline onto the title line rather than as a new header row
// (see renderHeader) specifically to avoid recalculating headerHeight/viewport sizing - this
// guards that renderHeader still renders without panicking once the notice is present.
func TestUpdateAvailableMsgRendersWithoutPanic(t *testing.T) {
	d := NewDashboard("1.0.2", 5001, true, nil, nil)
	updated, _ := d.Update(UpdateAvailableMsg{Version: "1.1.0"})
	out := updated.(Dashboard)
	out.width = 120
	out.height = 40
	out.ready = true

	header := out.renderHeader()
	if !strings.Contains(header, "update available: v1.1.0") {
		t.Errorf("header does not mention the available update:\n%s", header)
	}
}

func TestCaptureStateMsgClearsLANUrlsWhenEmpty(t *testing.T) {
	d := NewDashboard("v0", 5001, true, []string{"192.168.1.42"}, nil)
	if d.lanServerURL == "" {
		t.Fatal("expected non-empty lanServerURL after init with LAN address")
	}
	msg := CaptureStateMsg{
		Active:       []CaptureSummary{},
		LanAddresses: nil,
		Status:       "awaiting_interfaces",
	}
	updated, _ := d.Update(msg)
	out, ok := updated.(Dashboard)
	if !ok {
		t.Fatal("Update did not return Dashboard")
	}
	if out.lanServerURL != "" {
		t.Errorf("lanServerURL should be cleared, got %q", out.lanServerURL)
	}
	if out.lanWsURL != "" {
		t.Errorf("lanWsURL should be cleared, got %q", out.lanWsURL)
	}
	if out.captureStatus != "awaiting_interfaces" {
		t.Errorf("status=%q, want awaiting_interfaces", out.captureStatus)
	}
}

// @verified 2026-08-03: a WindowSizeMsg reporting a height at or below
// headerHeight+footerHeight+2 (a tiny terminal, or a degenerate first resize event some
// terminals send before the real one) used to drive viewportHeight to zero/negative,
// leaving d.ready=true with a broken viewport. bubbles/viewport v1.0.0's GotoBottom then
// panics with a slice-bounds-out-of-range on the very next LogMsg. Real crash reproduced via
// `go run ./cmd/radar -dev` in a small terminal window.
func TestTinyWindowSizeThenLogMsgDoesNotPanic(t *testing.T) {
	d := NewDashboard("v0", 5001, true, nil, nil)

	updated, _ := d.Update(tea.WindowSizeMsg{Width: 80, Height: 5})
	out, ok := updated.(Dashboard)
	if !ok {
		t.Fatal("Update did not return Dashboard")
	}
	if !out.ready {
		t.Fatal("expected the viewport to be marked ready after a WindowSizeMsg")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogMsg after a tiny WindowSizeMsg panicked: %v", r)
		}
	}()
	for range 5 {
		updated, _ = out.Update(LogMsg{Level: "INFO", Tag: "HTTP", Message: "test log line"})
		out = updated.(Dashboard)
	}
}

// @verified 2026-08-03: a zero-sized WindowSizeMsg (the most degenerate case) must not panic
// either - viewportHeight/viewportWidth are both clamped to a floor of 1.
func TestZeroWindowSizeThenLogMsgDoesNotPanic(t *testing.T) {
	d := NewDashboard("v0", 5001, true, nil, nil)

	updated, _ := d.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	out := updated.(Dashboard)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogMsg after a zero-sized WindowSizeMsg panicked: %v", r)
		}
	}()
	updated, _ = out.Update(LogMsg{Level: "INFO", Tag: "HTTP", Message: "test log line"})
	_ = updated.(Dashboard)
}

func TestFormatCaptureLine(t *testing.T) {
	cases := []struct {
		name string
		in   []CaptureSummary
		want string
	}{
		{"nil", nil, "(awaiting)"},
		{"empty slice", []CaptureSummary{}, "(awaiting)"},
		{"one", []CaptureSummary{{Description: "Wi-Fi", Address: "10.0.0.1"}}, "Wi-Fi (10.0.0.1)"},
		{"two", []CaptureSummary{
			{Description: "Wi-Fi", Address: "10.0.0.1"},
			{Description: "Ethernet", Address: "10.0.0.2"},
		}, "Wi-Fi (10.0.0.1), Ethernet (10.0.0.2)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCaptureLine(tc.in); got != tc.want {
				t.Errorf("formatCaptureLine(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
