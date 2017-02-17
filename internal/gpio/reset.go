// Package gpio configures and toggles GPIO pins using /sys/class/gpio.
package gpio

import (
	"io/ioutil"
	"log"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Configure configures pin as an output GPIO pin.
func Configure(pin string) error {
	if err := ioutil.WriteFile("/sys/class/gpio/export", []byte(pin), 0644); err != nil {
		if pe, ok := err.(*os.PathError); ok && pe.Err == syscall.EBUSY {
			// GPIO exports have already been configured, either we
			// ran already or the user knows what they are doing.
			// Fall-through to set the input/output status nevertheless:
		} else {
			return err
		}
	}
	if err := ioutil.WriteFile("/sys/class/gpio/gpio"+pin+"/direction", []byte("out"), 0644); err != nil {
		return err
	}
	return nil
}

// ResetUARTGW resets the UARTGW whose reset pin is connected to pin
// by holding the pin low for 150ms, flushing the pending UART data,
// then setting the pin high again.
func ResetUARTGW(pin string, uartfd uintptr) error {
	// Turn off device
	if err := ioutil.WriteFile("/sys/class/gpio/gpio"+pin+"/value", []byte("0"), 0644); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)

	// Flush all data in the input buffer
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uartfd, unix.TCFLSH, uintptr(syscall.TCIFLUSH)); err != 0 {
		log.Fatal(err)
	}

	// Turn on device
	if err := ioutil.WriteFile("/sys/class/gpio/gpio"+pin+"/value", []byte("1"), 0644); err != nil {
		return err
	}
	return nil
}
