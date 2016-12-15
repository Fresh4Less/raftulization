// +build !pixelsupport

package switchIO

import (
)

//NewSwitchIO factory for a switch object
func NewSwitchIO(pin int) chan int {

	return make(chan int)
}
