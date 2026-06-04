package uinput

import (
	"fmt"

	"github.com/pipe01/flydigictl/pkg/uinput/evdev"
)

type GamepadAxis struct {
	Name     string
	Min, Max int32
	Code     evdev.EvCode
}

type GamepadButton struct {
	Name string
	Code evdev.EvCode
}

type UinputGamepad struct {
	dev *evdev.InputDevice

	axes    []GamepadAxis
	buttons []GamepadButton
}

func NewUinputGamepad(name string, axes []GamepadAxis, buttons []GamepadButton) (*UinputGamepad, error) {
	abs := make([]evdev.EvCode, len(axes))
	absSetup := make([]evdev.UinputAbsSetup, len(axes))
	for i, a := range axes {
		abs[i] = a.Code
		absSetup[i] = evdev.UinputAbsSetup{
			Code: a.Code,
			AbsInfo: evdev.AbsInfo{
				Minimum: a.Min,
				Maximum: a.Max,
			},
		}
	}

	keys := make([]evdev.EvCode, len(buttons))
	for i, b := range buttons {
		keys[i] = b.Code
	}

	dev, err := evdev.CreateDevice(name, evdev.InputID{
		BusType: evdev.BUS_USB,
		Vendor:  0x04b4,
		Product: 0x2412,
		Version: 1,
	}, map[evdev.EvType][]evdev.EvCode{
		evdev.EV_KEY: keys,
		evdev.EV_ABS: abs,
		evdev.EV_FF:  {evdev.FF_RUMBLE},
		evdev.EV_SYN: {evdev.SYN_REPORT},
	}, absSetup)
	if err != nil {
		return nil, err
	}

	go func() {
		ev, err := dev.ReadOne()
		println(ev, err)
	}()

	return &UinputGamepad{
		dev:     dev,
		axes:    axes,
		buttons: buttons,
	}, nil
}

func (g *UinputGamepad) Close() error {
	if err := g.dev.Close(); err != nil {
		return fmt.Errorf("close device: %w", err)
	}

	if err := evdev.DestroyDevice(g.dev); err != nil {
		return fmt.Errorf("destroy device: %w", err)
	}

	return nil
}

func (g *UinputGamepad) Sync() error {
	return g.dev.WriteOne(&evdev.InputEvent{
		Type: evdev.EV_SYN,
		Code: evdev.SYN_REPORT,
	})
}

func (g *UinputGamepad) Key(index int, pressed bool) error {
	value := int32(0)
	if pressed {
		value = 1
	}

	return g.dev.WriteOne(&evdev.InputEvent{
		Type:  evdev.EV_KEY,
		Code:  g.buttons[index].Code,
		Value: value,
	})
}

func (g *UinputGamepad) Abs(index int, value int32) error {
	return g.dev.WriteOne(&evdev.InputEvent{
		Type:  evdev.EV_ABS,
		Code:  g.axes[index].Code,
		Value: value,
	})
}
