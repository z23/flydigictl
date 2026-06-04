package dinput

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/gousb"
	"github.com/pipe01/flydigictl/pkg/flydigi/products"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/internal"
	"github.com/pipe01/flydigictl/pkg/uinput"
	"github.com/pipe01/flydigictl/pkg/utils"

	"github.com/rs/zerolog/log"
)

const (
	packageLength    = 52
	ledPackageLength = 49
)

const (
	commandGetDongleVersion       = 17
	commandReadConfig             = 235
	commandGetDeviceInfoInAndroid = 236
	commandReadLEDConfig          = 229
)

type protocolDInput struct {
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint
	closer io.Closer

	msgch chan protocol.Message

	gpInfo *products.GamepadInfo

	configWriter *internal.ConfigWriter

	configReader, ledConfigReader *internal.ConfigReader
}

func Open() (prot protocol.Device, err error) {
	ctx := gousb.NewContext()

	var closers utils.MultiCloser
	defer func() {
		if err != nil {
			closers.Close()
		}
	}()

	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == 0x04b4 && desc.Product == 0x2412
	})
	if err != nil {
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	log.Debug().Int("count", len(devs)).Msg("found dinput usb devices")

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

	intf, err := cfg.Interface(2, 0)
	if err != nil {
		return nil, fmt.Errorf("open interface: %w", err)
	}
	closers.AddFunc(intf.Close)

	outep, err := intf.OutEndpoint(5)
	if err != nil {
		return nil, fmt.Errorf("open out endpoint: %w", err)
	}

	inep, err := intf.InEndpoint(3)
	if err != nil {
		return nil, fmt.Errorf("open in endpoint: %w", err)
	}

	gpInfo := products.Gamepads[products.GamepadVader3]

	p := &protocolDInput{
		in:              inep,
		out:             outep,
		closer:          &closers,
		msgch:           make(chan protocol.Message, 10),
		configReader:    internal.NewConfigReader(packageLength, 10),
		ledConfigReader: internal.NewConfigReader(ledPackageLength, 10),
		configWriter:    internal.NewConfigWriter(outep),
		gpInfo:          &gpInfo,
	}
	go p.readLoop()

	return p, nil
}

func (d *protocolDInput) Close() error {
	return d.closer.Close()
}

func (d *protocolDInput) Messages() <-chan protocol.Message {
	return d.msgch
}

func (d *protocolDInput) Inputs() ([]uinput.GamepadAxis, []uinput.GamepadButton) {
	return d.gpInfo.Axes, d.gpInfo.Buttons
}

func (d *protocolDInput) Manufacturer() string {
	return "Flydigi"
}

func (d *protocolDInput) Product() string {
	return d.gpInfo.Name
}

func (d *protocolDInput) readLoop() {
	buf := make([]byte, 32)

	defer close(d.msgch)

	for {
		n, err := d.in.Read(buf)
		if err != nil {
			break
		}

		data := buf[:n]

		msg, ok := d.resolveUsbData(data)
		if ok {
			d.msgch <- msg
		}
	}
}

func (d *protocolDInput) Send(ctx context.Context, cmd protocol.Command) error {
	switch cmd := cmd.(type) {
	case protocol.CommandGetDongleVersion:
		return d.sendCommand(commandGetDongleVersion)

	case protocol.CommandGetDeviceInfo:
		return d.sendCommand(commandGetDeviceInfoInAndroid)

	case protocol.CommandReadConfig:
		d.configReader.Reset()
		return d.sendCommand(commandReadConfig, cmd.ConfigID)

	case protocol.CommandReadLEDConfig:
		d.ledConfigReader.Reset()
		return d.sendCommand(commandReadLEDConfig, cmd.ConfigID)

	case protocol.CommandSendConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, false)

	case protocol.CommandSendLEDConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, true)

	default:
		return protocol.ErrUnknownCommand
	}
}

func (g *protocolDInput) sendConfig(ctx context.Context, data []byte, configID byte, isLED bool) error {
	var chunks [][]byte
	if isLED {
		chunks = getLEDConfigDataParcels(data, configID)
	} else {
		chunks = getConfigDataParcels(data, configID)
	}

	return g.configWriter.Send(ctx, chunks, 3, 3*time.Second)
}

func (d *protocolDInput) sendCommand(cmd byte, args ...byte) error {
	log.Debug().Uint8("cmd", cmd).Bytes("args", args).Msg("sending command")

	buf := make([]byte, 12)
	buf[0] = 5
	buf[1] = cmd
	copy(buf[2:], args)

	_, err := d.out.Write(buf)
	return err
}

func (d *protocolDInput) resolveUsbData(p []byte) (msg protocol.Message, ok bool) {
	log.Debug().Int("length", len(p)).Hex("data", p).Msg("got usb data")

	buttons, axes, ok := utils.ParseXboxGamepadInput(p)
	if ok {
		log.Debug().Msg("got input data")
		return protocol.MessageGamepadInput{
			Buttons: buttons,
			Axes:    axes,
		}, true
	}

	if p[15] == 235 {
		d.configReader.GotPackage(int(p[3]), p[5:15])

		if d.configReader.IsFinished() {
			// Wait for transmission to finish
			//TODO: Do it better
			time.Sleep(200 * time.Millisecond)

			return protocol.MessageGamepadConfigReadCB{
				Data: d.configReader.Data(),
			}, true
		}

		return nil, false
	}

	if p[15] == 229 {
		d.ledConfigReader.GotPackage(int(p[3]), p[5:15])

		if d.ledConfigReader.IsFinished() {
			// Wait for transmission to finish
			//TODO: Do it better
			time.Sleep(200 * time.Millisecond)

			return protocol.MessageLEDConfigReadCB{
				Data: d.ledConfigReader.Data(),
			}, true
		}

		return nil, false
	}

	if p[0] == 4 && p[1] == 17 {
		log.Debug().Str("handler", "HandleDongleInfo").Msg("got usb response")
		return protocol.MessageDongleInfo{
			FW_L: p[2],
			FW_H: p[3],
		}, true
	}

	if p[15] == 234 || p[15] == 231 || p[15] == 51 {
		d.configWriter.Ack(int(p[3]))

		return nil, false
	}

	if p[15] == 236 {
		return protocol.MessageGamePadInfo{
			DeviceID:         p[3],
			DeviceMac:        p[5:9],
			FW_L:             p[9],
			FW_H:             p[10],
			Battery:          p[11],
			MotionSensorType: p[14],
			CPUType:          p[12],
			ConnectionType:   p[13],
		}, true
	}

	if p[3] == 245 && p[4] == 1 {
		log.Debug().Str("handler", "ExtensionChipInfo").Msg("got usb response")
		return nil, false
	}

	if p[3] == 242 && p[4] == 3 {
		log.Debug().Str("handler", "ScreenInfo").Msg("got usb response")
		return nil, false
	}

	if p[3] == 242 && p[4] == 4 {
		log.Debug().Str("handler", "ScreenInfoSleepTime").Msg("got usb response")
		return nil, false
	}

	if p[0] == 4 && (p[1] == 240 || p[1] == 35) {
		log.Debug().Str("handler", "PicData").Msg("got usb response")
		return nil, false
	}

	if p[0] == 90 && p[1] == 165 && p[2] == 209 && p[4] == 0 {
		log.Debug().Str("handler", "WritePicData").Msg("got usb response")
		return nil, false
	}

	if p[3] == 250 && p[4] == 160 {
		log.Debug().Str("handler", "UUIDCB").Msg("got usb response")
		return nil, false
	}

	return nil, false
}
