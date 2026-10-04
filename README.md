# flydigictl

Modified 2026-10-04 by [z23](https://github.com/z23). This program is free software under the GNU General Public License version 3; see [LICENSE](LICENSE).

This is a modified copy of [pipe01/flydigictl](https://github.com/pipe01/flydigictl) by Felipe Martínez. Upstream copyright remains with the authors of that project. The modifications add `vader4-rgb`, a command that reads and writes the light bar on a Flydigi Vader 4 Pro without the D-Bus daemon. They also accept this pad's config header and its 32-byte XInput replies, and reattach the `xpad` driver after a lighting change.

## vader4-rgb

The controller has to be in XInput mode (hold the circle button and X until the indicator is white) or DInput mode (circle and A, blue indicator), over USB or the 2.4 GHz dongle. Switch mode and Bluetooth do not speak this protocol. The setting is stored on the controller.

```
go build -o vader4-rgb ./cmd/vader4rgb
```

```
vader4-rgb                  show the current lighting
vader4-rgb cyan             solid color
vader4-rgb '#00e5ff' -b 70  solid color at brightness 0–100
vader4-rgb off              light bar off
vader4-rgb stream           factory moving effect
vader4-rgb stream -s 0.5    same effect, speed from 0 to 1
```

Colors are `#RRGGBB`, `RRGGBB`, `#RGB`, or a name: red, green, blue, white, cyan, magenta, yellow, orange, purple, pink, black.

To use it without root, install the udev rule and replug the controller:

```
sudo install -m 644 udev/60-flydigi.rules /etc/udev/rules.d/60-flydigi.rules
sudo udevadm control --reload
sudo udevadm trigger --subsystem-match=usb
```

The XInput match requires the USB manufacturer string `Flydigi`, so a real Xbox 360 controller (`045e:028e`) is left alone. In XInput mode the Flydigi product string may say `VADER3`. The protocol device id 85 is a Vader 4 Pro.

`vader4-rgb` writes the LED configuration only. It does not rewrite buttons, sticks, or the rest of the onboard profile.

## Upstream daemon

The original `flydigictl` and `flydigid` programs are still in this tree. `flydigid` runs in the background as root, and `flydigictl` talks to it over D-Bus. Systemd starts the daemon when the D-Bus interface is requested.

Supported by upstream, and still present here:

- Vader 3 Pro

Not tested upstream, but probably works:

- Vader 3 Pro ONE PIECE
- Vader 3
- Vader 2
- Direwolf 2

### Installing the daemon

On Debian and Ubuntu, download the artifact from the [upstream actions run](https://github.com/pipe01/flydigictl/actions) and install the deb package. That build does not include `vader4-rgb`.

On other distributions, install `libusb-1.0` and [Go](https://go.dev/) 1.21.5 or newer, then run `sudo make install` to install `flydigictl` and `flydigid`.

Check the daemon with `flydigictl version`, then plug in the controller and run `flydigictl info`. Run `flydigictl help` for the rest of that command.

### Troubleshooting the daemon

- The controller must be in DInput or XInput mode, not Bluetooth or Switch mode.
- In XInput mode, `lsusb` shows `ID 045e:028e`.
- If the daemon cannot claim the device, unload `xpad` with `sudo modprobe -r xpad`. `flydigid` tries to do this itself.

## Disclaimer

THIS PROGRAM IS PROVIDED AS-IS AND MAKES NO EXPRESS OR IMPLIED WARRANTY OF ANY KIND. THE AUTHORS ARE NOT RESPONSIBLE FOR ANY POSSIBLE DAMAGE CAUSED TO THE DEVICE OR ANY ACCESSORIES.
