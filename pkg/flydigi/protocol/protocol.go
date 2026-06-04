package protocol

import (
	"context"
	"errors"

	"github.com/pipe01/flydigictl/pkg/uinput"
)

var (
	ErrUnknownCommand    = errors.New("unknown command type")
	ErrUnknownMessage    = errors.New("unknown message")
	ErrGamepadNotPresent = errors.New("gamepad not present")
)

type Message interface {
	message()
}

type Command interface {
	command()
}

type Protocol interface {
	Close() error

	Manufacturer() string
	Product() string

	Messages() <-chan Message
	Send(ctx context.Context, cmd Command) error

	Inputs() ([]uinput.GamepadAxis, []uinput.GamepadButton)
}
