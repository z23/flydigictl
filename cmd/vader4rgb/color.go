// Copyright (C) 2026 z23. This file is part of a modified version of
// pipe01/flydigictl, dated 2026-10-04, released under GPL-3.0-only.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

var colorNames = map[string][3]byte{
	"red":     {255, 0, 0},
	"green":   {0, 255, 0},
	"blue":    {0, 0, 255},
	"white":   {255, 255, 255},
	"cyan":    {0, 255, 255},
	"magenta": {255, 0, 255},
	"yellow":  {255, 255, 0},
	"orange":  {255, 80, 0},
	"purple":  {160, 0, 255},
	"pink":    {255, 40, 120},
	"black":   {0, 0, 0},
}

func parseColor(s string) (r, g, b byte, err error) {
	if rgb, ok := colorNames[strings.ToLower(s)]; ok {
		return rgb[0], rgb[1], rgb[2], nil
	}

	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("color %q is not a name or #RRGGBB", s)
	}
	n, convErr := strconv.ParseUint(hex, 16, 24)
	if convErr != nil {
		return 0, 0, 0, fmt.Errorf("color %q is not a name or #RRGGBB", s)
	}
	return byte(n >> 16), byte(n >> 8), byte(n), nil
}

func formatColor(r, g, b byte) string {
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
