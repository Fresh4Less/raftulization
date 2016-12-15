// +build !rpiosupport

package rotaryEncoderIO

import (
)

//NewRotaryEncoderIO factory for a rotaryEncoder object
func NewRotaryEncoderIO(pinA, pinB int) chan int {

	return make(chan int)
}

