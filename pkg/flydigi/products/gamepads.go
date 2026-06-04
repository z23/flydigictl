package products

import (
	"github.com/pipe01/flydigictl/pkg/uinput"
	"github.com/pipe01/flydigictl/pkg/uinput/evdev"
)

type GamepadModel int

const (
	GamepadVader3 GamepadModel = iota + 1
)

type GamepadInfo struct {
	Name    string
	Axes    []uinput.GamepadAxis
	Buttons []uinput.GamepadButton
}

var Gamepads = map[GamepadModel]GamepadInfo{
	GamepadVader3: {
		Name: "Vader 3",
		Axes: []uinput.GamepadAxis{
			{Name: "Left Joystick X", Code: evdev.ABS_X, Min: -32768, Max: 32767},
			{Name: "Left Joystick Y", Code: evdev.ABS_Y, Min: -32768, Max: 32767},
			{Name: "Right Joystick X", Code: evdev.ABS_RX, Min: -32768, Max: 32767},
			{Name: "Right Joystick Y", Code: evdev.ABS_RY, Min: -32768, Max: 32767},
			{Name: "Left Trigger", Code: evdev.ABS_Z, Min: 0, Max: 255},
			{Name: "Right Trigger", Code: evdev.ABS_RZ, Min: 0, Max: 255},
			{Name: "DPad X", Code: evdev.ABS_HAT0X, Min: -1, Max: 1},
			{Name: "DPad Y", Code: evdev.ABS_HAT0Y, Min: -1, Max: 1},
		},
		Buttons: []uinput.GamepadButton{
			{Name: "A", Code: evdev.BTN_A},
			{Name: "B", Code: evdev.BTN_B},
			{Name: "X", Code: evdev.BTN_X},
			{Name: "Y", Code: evdev.BTN_Y},
			{Name: "Start", Code: evdev.BTN_START},
			{Name: "Select", Code: evdev.BTN_SELECT},
			{Name: "Left Joystick", Code: evdev.BTN_THUMBL},
			{Name: "Right Joystick", Code: evdev.BTN_THUMBR},
			{Name: "Left Bumper", Code: evdev.BTN_TL},
			{Name: "Right Bumper", Code: evdev.BTN_TR},
			{Name: "C", Code: evdev.BTN_C},
			{Name: "Z", Code: evdev.BTN_Z},
			{Name: "Home", Code: evdev.BTN_MODE},
		},
	},
}
