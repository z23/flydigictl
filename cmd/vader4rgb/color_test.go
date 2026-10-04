// Copyright (C) 2026 z23. This file is part of a modified version of
// pipe01/flydigictl, dated 2026-10-04, released under GPL-3.0-only.

package main

import "testing"

func TestParseColor(t *testing.T) {
	cases := []struct {
		in      string
		r, g, b byte
	}{
		{"#00ffcc", 0, 255, 204},
		{"ff00aa", 255, 0, 170},
		{"#abc", 0xaa, 0xbb, 0xcc},
		{"red", 255, 0, 0},
		{"Cyan", 0, 255, 255},
	}
	for _, tc := range cases {
		r, g, b, err := parseColor(tc.in)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.in, err)
		}
		if r != tc.r || g != tc.g || b != tc.b {
			t.Fatalf("parse %s = %d %d %d, want %d %d %d", tc.in, r, g, b, tc.r, tc.g, tc.b)
		}
	}
	if _, _, _, err := parseColor("nope"); err == nil {
		t.Fatal("expected an error for an unknown color")
	}
}
