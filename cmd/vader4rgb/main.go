// Command vader4-rgb reads and writes the light bar on a Flydigi Vader 4 Pro.
//
// Copyright (C) 2026 z23. This file is part of a modified version of
// pipe01/flydigictl, dated 2026-10-04, released under GPL-3.0-only.
//
// The controller has to be in XInput mode (hold the circle button and X, white
// indicator) or DInput mode (circle and A, blue indicator), over USB or the
// 2.4 GHz dongle. The setting is stored on the controller.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pipe01/flydigictl/pkg/flydigi"
	"github.com/pipe01/flydigictl/pkg/flydigi/config"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/spf13/pflag"

	golog "log"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var deviceNames = map[int32]string{
	80: "Vader 3 Pro",
	81: "Vader 3 Pro",
	85: "Vader 4 Pro",
}

var ledModeNames = []string{"off", "stream", "breathing", "gradient", "feedback", "steady"}

func main() {
	brightness := pflag.IntP("brightness", "b", -1, "brightness from 0 to 100; with no color, only the brightness changes")
	speed := pflag.Float64P("speed", "s", -1, "stream effect speed from 0 to 1; default keeps the current speed")
	debug := pflag.Bool("debug", false, "print USB protocol logs")
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Set the RGB light bar on a Flydigi Vader 4 Pro from Linux.

Usage:
  vader4-rgb                     Show the current lighting
  vader4-rgb COLOR               Solid color
  vader4-rgb off                 Turn the light bar off
  vader4-rgb stream              Moving rainbow effect
  vader4-rgb COLOR -b 40         Solid color at a brightness from 0 to 100

COLOR is #RRGGBB, RRGGBB, #RGB, or a name: red, green, blue, white, cyan,
magenta, yellow, orange, purple, pink, black.

The controller must be in XInput mode (hold the circle button and X until the
indicator is white) or DInput mode (circle and A, blue indicator), connected
by USB or the 2.4 GHz dongle. The change is saved on the controller.

`)
		pflag.PrintDefaults()
	}
	pflag.Parse()

	golog.SetOutput(io.Discard)
	if *debug {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}

	if *brightness > 100 {
		exitf("brightness must be from 0 to 100")
	}

	args := pflag.Args()
	var (
		doOff    bool
		doStream bool
		doColor  bool
		r, g, b  byte
	)
	switch len(args) {
	case 0:
	case 1:
		switch strings.ToLower(args[0]) {
		case "off":
			doOff = true
		case "stream", "streamlined":
			doStream = true
		default:
			var err error
			r, g, b, err = parseColor(args[0])
			if err != nil {
				exitf("%s", err)
			}
			doColor = true
		}
	default:
		pflag.Usage()
		os.Exit(2)
	}
	if *speed > 1 {
		exitf("speed must be from 0 to 1")
	}
	changing := doOff || doColor || doStream || *brightness >= 0

	gp, err := flydigi.OpenGamepad()
	if err != nil {
		failOpen(err)
	}
	defer rebindXpad()
	defer gp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	info, err := gp.GetGamepadInfo(ctx)
	if err != nil {
		exitf("read controller: %s", err)
	}
	printInfo(info)

	led, err := gp.GetLEDConfig(ctx)
	if err != nil {
		exitf("read lighting: %s", err)
	}
	if !changing {
		printLED(led)
		return
	}

	if *brightness >= 0 {
		led.Light_scale = byte(*brightness)
	}
	if doOff {
		setOff(led)
	}
	if doColor {
		if *brightness < 0 && led.Light_scale == 0 {
			led.Light_scale = 100
			fmt.Println("Brightness was 0, so it was raised to 100.")
		}
		led.SetSteady(config.LedUnit{R: r, G: g, B: b})
	}
	if doStream {
		rate := float32(0.5)
		if *speed >= 0 {
			rate = float32(*speed)
		} else if led.Loop_time <= 100 {
			rate = float32(100-led.Loop_time) / 100
		}
		led.SetStreamlined(rate)
	}

	if err := gp.SaveLEDConfig(ctx, led); err != nil {
		exitf("write lighting: %s", err)
	}

	written, err := gp.GetLEDConfig(ctx)
	if err != nil {
		fmt.Printf("Sent. The controller did not send the new setting back: %s\n", err)
		return
	}
	fmt.Println("Saved.")
	printLED(written)
}

func printInfo(info *flydigi.FDGDeviceInfo) {
	name := deviceNames[info.DeviceId]
	if name == "" {
		name = "Flydigi controller"
	}
	link := "unknown"
	switch info.ConnectType {
	case flydigi.FDGConncetWired:
		link = "wired"
	case flydigi.FDGConncetWireless:
		link = "wireless"
	}
	fmt.Printf("Controller : %s (id %d)\n", name, info.DeviceId)
	fmt.Printf("Firmware   : %s\n", empty(info.FirmwareVersion))
	fmt.Printf("Link       : %s\n", link)
	if info.DeviceId != 85 && info.DeviceId != 0 {
		fmt.Println("This is not reporting as a Vader 4 Pro. The lighting command is the same Flydigi protocol.")
	}
}

func printLED(led *config.NewLedConfigBean) {
	mode := fmt.Sprintf("mode %d", led.LedMode)
	if int(led.LedMode) < len(ledModeNames) {
		mode = ledModeNames[led.LedMode]
	}
	fmt.Printf("Lighting   : %s\n", mode)
	fmt.Printf("Brightness : %d\n", led.Light_scale)
	if led.LedMode == config.LedModeStreamlined && led.Loop_time <= 100 {
		fmt.Printf("Speed      : %.2f\n", float32(100-led.Loop_time)/100)
	}

	n := int(led.Rgb_num)
	if n <= 0 || n > len(led.LedGroups) {
		n = len(led.LedGroups)
	}
	if n > 5 {
		n = 5
	}
	if n == 0 {
		return
	}
	parts := make([]string, 0, n)
	for _, g := range led.LedGroups[:n] {
		if len(g.Units) == 0 || g.Units[0] == nil {
			parts = append(parts, "#000000")
			continue
		}
		u := g.Units[0]
		parts = append(parts, formatColor(u.R, u.G, u.B))
	}
	fmt.Printf("Zones      : %s\n", strings.Join(parts, " "))
}

func setOff(led *config.NewLedConfigBean) {
	led.LedMode = config.LedModeOff
	n := int(led.Rgb_num)
	if n <= 0 || n > len(led.LedGroups) {
		n = len(led.LedGroups)
	}
	for _, g := range led.LedGroups[:n] {
		if len(g.Units) == 0 || g.Units[0] == nil {
			continue
		}
		g.Units[0].R, g.Units[0].G, g.Units[0].B = 0, 0, 0
	}
}

func empty(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func failOpen(err error) {
	fmt.Fprintf(os.Stderr, "vader4-rgb: %s\n", err)
	msg := strings.ToLower(err.Error())
	if errors.Is(err, protocol.ErrGamepadNotPresent) || strings.Contains(msg, "not present") {
		fmt.Fprintf(os.Stderr, `
No Flydigi controller was found. Plug it in over USB or the 2.4 GHz dongle.
Hold the circle button and X for XInput mode (white indicator), or the circle
button and A for DInput mode (blue indicator).
`)
		os.Exit(1)
	}
	if strings.Contains(msg, "access") || strings.Contains(msg, "permission") || strings.Contains(msg, "busy") || strings.Contains(msg, "claim") || strings.Contains(msg, "detach") {
		fmt.Fprintf(os.Stderr, `
The controller is connected, but this user cannot claim it yet. Install
udev/60-flydigi.rules from this repository, then unplug and replug the controller:

  sudo install -m 644 udev/60-flydigi.rules /etc/udev/rules.d/60-flydigi.rules
  sudo udevadm control --reload
  sudo udevadm trigger --subsystem-match=usb

Until that is installed, prefix the command with sudo.
`)
	}
	os.Exit(1)
}

func rebindXpad() {
	dirs, err := filepath.Glob("/sys/bus/usb/devices/*")
	if err != nil {
		return
	}
	for _, dir := range dirs {
		vendor := sysText(filepath.Join(dir, "idVendor"))
		product := sysText(filepath.Join(dir, "idProduct"))
		if vendor != "045e" || product != "028e" {
			continue
		}
		if !strings.Contains(strings.ToLower(sysText(filepath.Join(dir, "manufacturer"))), "flydigi") {
			continue
		}
		ifaces, _ := filepath.Glob(dir + ":*")
		for _, iface := range ifaces {
			if _, err := os.Stat(filepath.Join(iface, "driver")); err == nil {
				continue
			}
			_ = os.WriteFile("/sys/bus/usb/drivers/xpad/bind", []byte(filepath.Base(iface)), 0o200)
		}
	}
}

func sysText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vader4-rgb: "+format+"\n", args...)
	os.Exit(1)
}
