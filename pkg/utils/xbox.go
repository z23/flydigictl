package utils

import (
	"encoding/binary"
)

func ParseXboxGamepadInput(p []byte) (buttons []bool, axes []int32, ok bool) {
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

		buttons = []bool{
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
		}
		axes = []int32{
			int32(int16(binary.LittleEndian.Uint16(p[6:]))),
			-int32(int16(binary.LittleEndian.Uint16(p[8:]))),
			int32(int16(binary.LittleEndian.Uint16(p[10:]))),
			-int32(int16(binary.LittleEndian.Uint16(p[12:]))),
			int32(p[4]),
			int32(p[5]),
			dpadx,
			dpady,
		}
		ok = true
		return
	}

	return nil, nil, false
}
