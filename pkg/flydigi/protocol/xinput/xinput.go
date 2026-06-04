package xinput

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/internal"
	"github.com/pipe01/flydigictl/pkg/uinput"
	"github.com/pipe01/flydigictl/pkg/uinput/evdev"
	"github.com/pipe01/flydigictl/pkg/utils"

	"github.com/google/gousb"
	"github.com/rs/zerolog/log"
)

const (
	packageLength    = 52
	ledPackageLength = 49
)

const (
	commandGetDongleVersion = 17
	commandReadConfig       = 33
	commandGetDeviceInfo    = 16
	commandReadLEDConfig    = 38
)

type protocolXInput struct {
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint
	closer io.Closer

	manufacturer, product string

	isClosed atomic.Bool

	msgch chan protocol.Message

	configReader, ledConfigReader *internal.ConfigReader

	configWriter *internal.ConfigWriter

	axes    []uinput.GamepadAxis
	buttons []uinput.GamepadButton
}

func Open() (protocol.Protocol, error) {
	ctx := gousb.NewContext()

	var closers utils.MultiCloser

	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == 0x045e && desc.Product == 0x028e
	})
	if err != nil {
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	log.Debug().Int("count", len(devs)).Msg("found xinput usb devices")

	if len(devs) == 0 {
		return nil, protocol.ErrGamepadNotPresent
	}

	dev := devs[0]
	closers.AddCloser(dev)

	manufacturer, err := dev.Manufacturer()
	if err != nil {
		return nil, fmt.Errorf("get manufacturer: %w", err)
	}
	product, err := dev.Product()
	if err != nil {
		return nil, fmt.Errorf("get manufacturer: %w", err)
	}
	serialNumber, err := dev.SerialNumber()
	if err != nil {
		return nil, fmt.Errorf("get serial number: %w", err)
	}

	log.Info().Str("manufacturer", manufacturer).Str("product", product).Str("serial", serialNumber).Msg("found gamepad")

	if err := dev.SetAutoDetach(true); err != nil {
		log.Err(err).Msg("failed to enable kernel driver auto detach mode")
	}

	cfg, err := dev.Config(1)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	closers.AddCloser(cfg)

	intf, err := cfg.Interface(0, 0)
	if err != nil {
		return nil, fmt.Errorf("open interface: %w", err)
	}
	closers.AddFunc(intf.Close)

	outep, err := intf.OutEndpoint(5)
	if err != nil {
		return nil, fmt.Errorf("open out endpoint: %w", err)
	}

	inep, err := intf.InEndpoint(1)
	if err != nil {
		return nil, fmt.Errorf("open in endpoint: %w", err)
	}

	axes := []uinput.GamepadAxis{
		{Name: "Left Joystick X", Code: evdev.ABS_X, Min: -32768, Max: 32767},
		{Name: "Left Joystick Y", Code: evdev.ABS_Y, Min: -32768, Max: 32767},
		{Name: "Right Joystick X", Code: evdev.ABS_RX, Min: -32768, Max: 32767},
		{Name: "Right Joystick Y", Code: evdev.ABS_RY, Min: -32768, Max: 32767},
		{Name: "Left Trigger", Code: evdev.ABS_Z, Min: 0, Max: 255},
		{Name: "Right Trigger", Code: evdev.ABS_RZ, Min: 0, Max: 255},
		{Name: "DPad X", Code: evdev.ABS_HAT0X, Min: -1, Max: 1},
		{Name: "DPad Y", Code: evdev.ABS_HAT0Y, Min: -1, Max: 1},
	}
	buttons := []uinput.GamepadButton{
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
	}

	p := &protocolXInput{
		in:              inep,
		out:             outep,
		closer:          &closers,
		manufacturer:    manufacturer,
		product:         product,
		msgch:           make(chan protocol.Message, 10),
		configReader:    internal.NewConfigReader(packageLength, 10),
		ledConfigReader: internal.NewConfigReader(ledPackageLength, 10),
		configWriter:    internal.NewConfigWriter(outep),
		axes:            axes,
		buttons:         buttons,
	}
	go p.readLoop()

	return p, nil
}

func (d *protocolXInput) Inputs() ([]uinput.GamepadAxis, []uinput.GamepadButton) {
	return d.axes, d.buttons
}

func (d *protocolXInput) Manufacturer() string {
	return d.manufacturer
}

func (d *protocolXInput) Product() string {
	return d.product
}

func (d *protocolXInput) Close() error {
	if d.isClosed.Swap(true) {
		return nil
	}

	return d.closer.Close()
}

func (d *protocolXInput) Messages() <-chan protocol.Message {
	return d.msgch
}

func (d *protocolXInput) readLoop() {
	buf := make([]byte, 100)

	defer close(d.msgch)

	log.Debug().Msg("starting xinput read loop")

	for {
		n, err := d.in.Read(buf)
		if err != nil {
			log.Err(err).Msg(fmt.Sprintf("%#v", err))

			if status, ok := err.(gousb.TransferStatus); !ok || status != gousb.TransferNoDevice {
				log.Err(err).Msg("failed to read data from usb")
			}

			break
		}

		data := buf[:n]

		msg, ok := d.resolveUsbData(data)
		if ok {
			// log.Debug().Any("msg", msg).Msg("msg")
			d.msgch <- msg
		}
	}

	log.Debug().Msg("xinput read loop exited")
}

func (d *protocolXInput) resolveUsbData(p []byte) (protocol.Message, bool) {
	if len(p) == 32 && p[0] == 0 && p[1] == 20 {
		// p[14], p[15] and p[16] are the gyroscope axes, but there are no evdev codes for them.
		// We could create a second virtual gamepad only for gyroscope but that's probably confusing for the user.

		var dpadx, dpady int32
		if (p[2]>>0)&1 != 0 {
			dpady -= 1
		}
		if (p[2]>>1)&1 != 0 {
			dpady += 1
		}
		if (p[2]>>2)&1 != 0 {
			dpadx -= 1
		}
		if (p[2]>>3)&1 != 0 {
			dpadx += 1
		}

		return protocol.MessageGamepadInput{
			Buttons: []bool{
				(p[3]>>4)&1 != 0,
				(p[3]>>5)&1 != 0,
				(p[3]>>6)&1 != 0,
				(p[3]>>7)&1 != 0,
				(p[2]>>4)&1 != 0,
				(p[2]>>5)&1 != 0,
				(p[18]>>6)&1 != 0,
				(p[18]>>7)&1 != 0,
				(p[3]>>0)&1 != 0,
				(p[3]>>1)&1 != 0,
				(p[19]>>0)&1 != 0,
				(p[19]>>1)&1 != 0,
				(p[3]>>2)&1 != 0,
			},
			Axes: []int32{
				int32(int16(binary.LittleEndian.Uint16(p[6:]))),
				-int32(int16(binary.LittleEndian.Uint16(p[8:]))),
				int32(int16(binary.LittleEndian.Uint16(p[10:]))),
				-int32(int16(binary.LittleEndian.Uint16(p[12:]))),
				int32(p[4]),
				int32(p[5]),
				dpadx,
				dpady,
			},
		}, true
	} else if p[14] == 0xA5 {
		switch p[15] {
		case 16:
			return protocol.MessageGamePadInfo{
				DeviceID:         p[16],
				DeviceMac:        p[17:21],
				FW_L:             p[21],
				FW_H:             p[22],
				Battery:          p[23],
				CPUType:          p[24],
				ConnectionType:   p[25],
				MotionSensorType: p[26],
			}, true

		case 17:
			return protocol.MessageDongleInfo{
				FW_L: p[16],
				FW_H: p[17],
			}, true

		case 32:
			// HandleGamepadConfigId

		case 34:
			// HandleGamepadConfigReadCB
			d.configReader.GotPackage(int(p[16]), p[17:28])

			if d.configReader.IsFinished() {
				time.Sleep(200 * time.Millisecond)
				return protocol.MessageGamepadConfigReadCB{
					Data: d.configReader.Data(),
				}, true
			}

		case 35, 37: // HandleStartWriteGamepadConfig
			d.configWriter.Ack(0)

		case 36: // HandleWriteGamepadConfigCBK
			d.configWriter.Ack(int(p[16]))

		case 39:
			// HandleLedConfigReadCB
			d.ledConfigReader.GotPackage(int(p[16]), p[17:28])

			if d.ledConfigReader.IsFinished() {
				time.Sleep(200 * time.Millisecond)
				return protocol.MessageLEDConfigReadCB{
					Data: d.ledConfigReader.Data(),
				}, true
			}

		case 41: // HandleWriteLEDConfigCBK
			d.configWriter.Ack(int(p[16]))

		case 42: // HandleStartWriteLEDConfig
			d.configWriter.Ack(0)
		}
	}

	return nil, false
}

func (d *protocolXInput) Send(ctx context.Context, cmd protocol.Command) error {
	switch cmd := cmd.(type) {
	case protocol.CommandGetDongleVersion:
		return d.sendCommand(ctx, commandGetDongleVersion)

	case protocol.CommandGetDeviceInfo:
		return d.sendCommand(ctx, commandGetDeviceInfo)

	case protocol.CommandReadConfig:
		d.configReader.Reset()
		return d.sendCommand(ctx, commandReadConfig, cmd.ConfigID)

	case protocol.CommandReadLEDConfig:
		d.ledConfigReader.Reset()
		return d.sendCommand(ctx, commandReadLEDConfig, cmd.ConfigID)

	case protocol.CommandSendConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, false)

	case protocol.CommandSendLEDConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, true)

	default:
		return protocol.ErrUnknownCommand
	}
}

func (d *protocolXInput) sendCommand(ctx context.Context, cmd byte, args ...byte) error {
	log.Debug().Uint8("cmd", cmd).Bytes("args", args).Msg("sending command")

	pkg := make([]byte, 15)
	pkg[0] = 165
	pkg[1] = cmd
	copy(pkg[2:], args)

	_, err := d.out.WriteContext(ctx, crcData(pkg))
	return err
}

func (g *protocolXInput) sendConfig(ctx context.Context, data []byte, configID byte, isLED bool) error {
	var chunks [][]byte
	if isLED {
		chunks = getLEDConfigDataParcels(data, configID)
	} else {
		chunks = getConfigDataParcels(data, configID)
	}

	return g.configWriter.Send(ctx, chunks, 3, 3*time.Second)
}
