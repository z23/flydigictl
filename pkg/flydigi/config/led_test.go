// Copyright (C) 2026 z23. This file is part of a modified version of
// pipe01/flydigictl, dated 2026-10-04, released under GPL-3.0-only.

package config

import "testing"

func TestStreamlinedPreset(t *testing.T) {
	b := &NewLedConfigBean{}
	b.SetStreamlined(0.5)
	if b.LedMode != LedModeStreamlined {
		t.Fatalf("mode %d", b.LedMode)
	}
	if b.Loop_time != 50 {
		t.Fatalf("loop time %d", b.Loop_time)
	}
	if len(b.LedGroups) < 5 || len(b.LedGroups[0].Units) != 10 {
		t.Fatalf("groups %d units %d", len(b.LedGroups), len(b.LedGroups[0].Units))
	}
	u := b.LedGroups[0].Units[0]
	if u.R != 50 || u.G != 50 || u.B != 0 {
		t.Fatalf("first frame #%02X%02X%02X", u.R, u.G, u.B)
	}
}

func TestStreamlinedSpeedRoundTrip(t *testing.T) {
	b := &NewLedConfigBean{Loop_time: 16}
	rate := float32(100-b.Loop_time) / 100
	b.SetStreamlined(rate)
	if b.Loop_time != 16 {
		t.Fatalf("loop time %d", b.Loop_time)
	}
}
