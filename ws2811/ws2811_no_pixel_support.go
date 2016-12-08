// +build !pixelsupport

package ws2811
import (
)

func Init(gpioPin int, ledCount int, brightness int) error {
	return nil
}

func Fini() {
}

func Render() error {
	return nil
}

func Wait() error {
	return nil
}

func SetLed(index int, value uint32) {
}

func Clear() {
}

func SetBitmap(a []uint32) {
}
