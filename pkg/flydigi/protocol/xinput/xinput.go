package xinput

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/internal"
	"github.com/pipe01/flydigictl/pkg/utils"

	"github.com/google/gousb"
	"github.com/rs/zerolog/log"
	"pault.ag/go/modprobe"
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

	isClosed atomic.Bool

	msgch chan protocol.Message

	configReader, ledConfigReader *internal.ConfigReader

	configWriter *internal.ConfigWriter
}

func isFlydigiXInput(d *gousb.Device) bool {
	mfr, merr := d.Manufacturer()
	prod, perr := d.Product()
	if merr != nil && perr != nil {
		return false
	}
	blob := strings.ToLower(mfr + "\n" + prod)
	return strings.Contains(blob, "flydigi") || strings.Contains(blob, "vader")
}

// attachKernelDriver asks usbfs to bind the interface back to its kernel driver.
// libusb's auto-detach does not reattach when the driver was removed before the
// interface was claimed, which is what gousb does inside Config().
func attachKernelDriver(bus, addr, iface int) {
	path := fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, addr)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("open usbfs to reattach driver")
		return
	}
	defer f.Close()

	type usbdevfsIoctl struct {
		ifno      int32
		ioctlCode int32
		data      uint64
	}
	const (
		usbdevfsIoctlReq = 0xc0105512
		usbdevfsConnect  = 0x5517
	)
	cmd := usbdevfsIoctl{ifno: int32(iface), ioctlCode: usbdevfsConnect}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), usbdevfsIoctlReq, uintptr(unsafe.Pointer(&cmd)))
	if errno != 0 && errno != syscall.EBUSY {
		log.Error().Err(errno).Str("path", path).Msg("reattach kernel driver")
	}
}

func reloadModule(name string) {
	if name == "" {
		return
	}
	if err := modprobe.Load(name, ""); err == nil {
		return
	}
	if out, err := exec.Command("modprobe", name).CombinedOutput(); err != nil {
		log.Err(err).Str("module", name).Str("output", strings.TrimSpace(string(out))).Msg("failed to load xpad module")
	}
}

func Open() (protocol.Protocol, error) {
	ctx := gousb.NewContext()

	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == 0x045e && desc.Product == 0x028e
	})
	if err != nil && len(devs) == 0 {
		ctx.Close()
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	log.Debug().Int("count", len(devs)).Msg("found xinput usb devices")

	var dev *gousb.Device
	for _, d := range devs {
		if dev == nil && isFlydigiXInput(d) {
			dev = d
			continue
		}
		d.Close()
	}
	if dev == nil {
		ctx.Close()
		return nil, protocol.ErrGamepadNotPresent
	}

	// Detach xpad from this device only. libusb reattaches it when the interface is released.
	if err := dev.SetAutoDetach(true); err != nil {
		log.Debug().Err(err).Msg("set auto detach")
	}

	cfg, err := dev.Config(1)
	var xpadModule string
	if err != nil {
		for _, mod := range []string{"xpad", "xpad_noone"} {
			if modprobe.Remove(mod) == nil {
				xpadModule = mod
				log.Debug().Str("module", mod).Msg("unloaded xpad module")
				break
			}
		}
		if xpadModule == "" {
			dev.Close()
			ctx.Close()
			return nil, fmt.Errorf("open configuration: %w", err)
		}
		cfg, err = dev.Config(1)
		if err != nil {
			reloadModule(xpadModule)
			dev.Close()
			ctx.Close()
			return nil, fmt.Errorf("open configuration: %w", err)
		}
	}

	intf, err := cfg.Interface(0, 0)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctx.Close()
		reloadModule(xpadModule)
		return nil, fmt.Errorf("open interface: %w", err)
	}

	bus, addr := dev.Desc.Bus, dev.Desc.Address
	var closers utils.MultiCloser
	closers.AddFunc(func() {
		attachKernelDriver(bus, addr, 0)
		reloadModule(xpadModule)
	})
	closers.AddCloser(dev)
	closers.AddCloser(cfg)
	closers.AddFunc(intf.Close)

	outep, err := intf.OutEndpoint(5)
	if err != nil {
		closers.Close()
		return nil, fmt.Errorf("open out endpoint: %w", err)
	}

	inep, err := intf.InEndpoint(1)
	if err != nil {
		closers.Close()
		return nil, fmt.Errorf("open in endpoint: %w", err)
	}

	p := &protocolXInput{
		in:              inep,
		out:             outep,
		closer:          &closers,
		msgch:           make(chan protocol.Message, 10),
		configReader:    internal.NewConfigReader(packageLength, 10),
		ledConfigReader: internal.NewConfigReader(ledPackageLength, 10),
		configWriter:    internal.NewConfigWriter(outep),
	}
	go p.readLoop()

	return p, nil
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

	for {
		n, err := d.in.Read(buf)
		if err != nil {
			if status, ok := err.(gousb.TransferStatus); !ok || status != gousb.TransferNoDevice {
				log.Err(err).Msg("failed to read data from usb")
			}

			break
		}

		data := buf[:n]
		if bytes.Contains(data, []byte{0xa5}) {
			log.Debug().Int("n", n).Str("hex", hex.EncodeToString(data)).Msg("usb in")
		}

		msg, ok := d.resolveUsbData(data)
		if ok {
			d.msgch <- msg
		}
	}
}

func (d *protocolXInput) resolveUsbData(p []byte) (protocol.Message, bool) {
	// Input reports share this endpoint. Flydigi replies are marked 0xA5 at byte 14.
	if len(p) < 28 || p[14] != 0xA5 {
		return nil, false
	}
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
