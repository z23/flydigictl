package uinput

import (
	"errors"
	"fmt"
	"io"

	"github.com/pipe01/flydigictl/pkg/uinput/evdev"
	"github.com/rs/zerolog/log"
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
		evdev.EV_FF:  {evdev.FF_CONSTANT},
		evdev.EV_SYN: {evdev.SYN_REPORT},
	}, absSetup)
	if err != nil {
		return nil, err
	}

	ug := &UinputGamepad{
		dev:     dev,
		axes:    axes,
		buttons: buttons,
	}
	go ug.readLoop()

	return ug, nil
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

func (g *UinputGamepad) readLoop() {
	for {
		ev, err := g.dev.ReadOne()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Err(err).Msg("failed to read uinput events")
			}

			break
		}

		log.Debug().Uint16("type", uint16(ev.Type)).Uint16("code", uint16(ev.Code)).Int32("value", ev.Value).Msg("got uinput event")

		switch ev.Type {
		case evdev.EV_FF:
			err = g.dev.WriteOne(&evdev.InputEvent{
				Type:  evdev.EV_FF_STATUS,
				Code:  ev.Code,
				Value: evdev.FF_STATUS_PLAYING,
			})
			if err != nil {
				log.Err(err).Msg("failed to write FF status")
			}
		}
	}

	log.Debug().Msg("uinput read loop exited")
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
