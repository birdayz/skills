// keysend — inject keystrokes via /dev/uinput (no X, works on wlroots/sway).
// Usage:
//   keysend type:"hello world"   -> types the literal text
//   keysend tap:enter            -> taps a named key
//   keysend sleep:300            -> sleep ms
// Args run in order, all on ONE virtual device. Example:
//   keysend tap:t type:"/karoland build" tap:enter
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	uiSetEvbit  = 0x40045564
	uiSetKeybit = 0x40045565
	uiDevSetup  = 0x405C5503
	uiDevCreate = 0x5501
	uiDevDest   = 0x5502
	evSyn       = 0
	evKey       = 1
	keyLShift   = 42
)

// rune -> (linux keycode, needsShift)
var keymap = map[rune][2]int{
	'a': {30, 0}, 'b': {48, 0}, 'c': {46, 0}, 'd': {32, 0}, 'e': {18, 0}, 'f': {33, 0},
	'g': {34, 0}, 'h': {35, 0}, 'i': {23, 0}, 'j': {36, 0}, 'k': {37, 0}, 'l': {38, 0},
	'm': {50, 0}, 'n': {49, 0}, 'o': {24, 0}, 'p': {25, 0}, 'q': {16, 0}, 'r': {19, 0},
	's': {31, 0}, 't': {20, 0}, 'u': {22, 0}, 'v': {47, 0}, 'w': {17, 0}, 'x': {45, 0},
	'y': {21, 0}, 'z': {44, 0},
	'1': {2, 0}, '2': {3, 0}, '3': {4, 0}, '4': {5, 0}, '5': {6, 0},
	'6': {7, 0}, '7': {8, 0}, '8': {9, 0}, '9': {10, 0}, '0': {11, 0},
	' ': {57, 0}, '-': {12, 0}, '=': {13, 0}, '/': {53, 0}, '.': {52, 0}, ',': {51, 0},
	';': {39, 0}, '\'': {40, 0}, '[': {26, 0}, ']': {27, 0}, '\\': {43, 0}, '`': {41, 0},
	'_': {12, 1}, '+': {13, 1}, ':': {39, 1}, '?': {53, 1}, '!': {2, 1}, '@': {3, 1},
	'#': {4, 1}, '$': {5, 1}, '%': {6, 1}, '^': {7, 1}, '&': {8, 1}, '*': {9, 1},
	'(': {10, 1}, ')': {11, 1}, '"': {40, 1}, '<': {51, 1}, '>': {52, 1}, '{': {26, 1}, '}': {27, 1},
	'~': {41, 1}, '|': {43, 1},
	// uppercase = same keycode as lowercase, with shift
	'A': {30, 1}, 'B': {48, 1}, 'C': {46, 1}, 'D': {32, 1}, 'E': {18, 1}, 'F': {33, 1},
	'G': {34, 1}, 'H': {35, 1}, 'I': {23, 1}, 'J': {36, 1}, 'K': {37, 1}, 'L': {38, 1},
	'M': {50, 1}, 'N': {49, 1}, 'O': {24, 1}, 'P': {25, 1}, 'Q': {16, 1}, 'R': {19, 1},
	'S': {31, 1}, 'T': {20, 1}, 'U': {22, 1}, 'V': {47, 1}, 'W': {17, 1}, 'X': {45, 1},
	'Y': {21, 1}, 'Z': {44, 1},
}

var named = map[string]int{
	"enter": 28, "esc": 1, "tab": 15, "backspace": 14, "space": 57, "slash": 53,
	"up": 103, "down": 108, "left": 105, "right": 106, "f3": 61, "lshift": 42,
	"e": 18, "t": 20, "w": 17, "a": 30, "s": 31, "d": 32,
}

var fd int

func ioctl(req, arg uintptr) {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, arg)
	if e != 0 {
		fmt.Fprintf(os.Stderr, "ioctl 0x%x failed: %v\n", req, e)
		os.Exit(1)
	}
}

func emit(typ, code, val int) {
	var b [24]byte // input_event: 16B time + u16 type + u16 code + i32 value
	binary.LittleEndian.PutUint16(b[16:], uint16(typ))
	binary.LittleEndian.PutUint16(b[18:], uint16(code))
	binary.LittleEndian.PutUint32(b[20:], uint32(int32(val)))
	syscall.Write(fd, b[:])
}

func tapCode(code int, shift bool) {
	if shift {
		emit(evKey, keyLShift, 1)
		emit(evSyn, 0, 0)
	}
	emit(evKey, code, 1)
	emit(evSyn, 0, 0)
	time.Sleep(8 * time.Millisecond)
	emit(evKey, code, 0)
	emit(evSyn, 0, 0)
	if shift {
		emit(evKey, keyLShift, 0)
		emit(evSyn, 0, 0)
	}
	time.Sleep(12 * time.Millisecond)
}

func main() {
	var err error
	fd, err = syscall.Open("/dev/uinput", syscall.O_WRONLY, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open /dev/uinput:", err)
		os.Exit(1)
	}
	defer syscall.Close(fd)

	ioctl(uiSetEvbit, evKey)
	ioctl(uiSetEvbit, evSyn)
	for kc := 1; kc < 128; kc++ {
		ioctl(uiSetKeybit, uintptr(kc))
	}

	// uinput_setup: input_id(8) + name[80] + ff_effects_max(4) = 92 bytes
	var setup [92]byte
	binary.LittleEndian.PutUint16(setup[0:], 0x03)   // BUS_USB
	binary.LittleEndian.PutUint16(setup[2:], 0x1234) // vendor
	binary.LittleEndian.PutUint16(setup[4:], 0x5678) // product
	binary.LittleEndian.PutUint16(setup[6:], 0x0001) // version
	copy(setup[8:], "karoland-keysend")
	ioctl(uiDevSetup, uintptr(unsafe.Pointer(&setup)))
	ioctl(uiDevCreate, 0)
	time.Sleep(250 * time.Millisecond) // let sway/libinput register the device

	for _, arg := range os.Args[1:] {
		switch {
		case strings.HasPrefix(arg, "type:"):
			for _, r := range arg[len("type:"):] {
				m, ok := keymap[r]
				if !ok {
					fmt.Fprintf(os.Stderr, "no keymap for %q, skipping\n", r)
					continue
				}
				tapCode(m[0], m[1] == 1)
			}
		case strings.HasPrefix(arg, "tap:"):
			name := arg[len("tap:"):]
			if code, ok := named[name]; ok {
				tapCode(code, false)
			} else if m, ok := keymap[[]rune(name)[0]]; ok && len(name) == 1 {
				tapCode(m[0], m[1] == 1)
			} else {
				fmt.Fprintf(os.Stderr, "unknown key %q\n", name)
			}
		case strings.HasPrefix(arg, "sleep:"):
			ms, _ := strconv.Atoi(arg[len("sleep:"):])
			time.Sleep(time.Duration(ms) * time.Millisecond)
		default:
			fmt.Fprintf(os.Stderr, "bad arg %q\n", arg)
		}
	}

	time.Sleep(80 * time.Millisecond)
	ioctl(uiDevDest, 0)
	fmt.Println("ok")
}
