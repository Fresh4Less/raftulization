// +build pixelsupport

package rotaryEncoderIO

import (
	"fmt"
	"time"

	"github.com/stianeikeland/go-rpio"
)

var (
	initialized     = false
	leftNum         = 33
	rightNum        = 18
	refreshRate     = 1 //Milliseconds
	watchedSwitches = []*rotaryEncoder{}
)

func initialize() bool {

	if err := rpio.Open(); err != nil {
		fmt.Println(err)
		return false
	}

	initialized = true
	go watchChanges()

	return true
}

//NewRotaryEncoderIO factory for a rotaryEncoder object
func NewRotaryEncoderIO(pinA, pinB int) chan int {

	if !initialized {
		success := initialize()
		if !success {
			return nil
		}
	}

	s := new(rotaryEncoder)

	s.pinA = rpio.Pin(pinA)
	s.pinB = rpio.Pin(pinB)

	s.pinA.Input()
	s.pinB.Input()
	s.pinA.PullUp()
	s.pinB.PullUp()

	s.prevA = 1
	s.prevB = 1

	s.seq = 0

	s.updCh = make(chan int)

	watchedSwitches = append(watchedSwitches, s)

	return s.updCh
}

type rotaryEncoder struct {
	pinA  rpio.Pin
	pinB  rpio.Pin
	prevA int
	prevB int
	seq   int

	updCh chan int
}

func watchChanges() {

	defer rpio.Close()

	for true {

		for _, sw := range watchedSwitches {

			a := int(sw.pinA.Read())
			b := int(sw.pinB.Read())

			if a != sw.prevA || b != sw.prevB {

				sw.prevA = a
				sw.prevB = b

				if a == 1 && b == 1 {
					if sw.seq == leftNum {
						go func(s *rotaryEncoder) {
							s.updCh <- 0
						}(sw)
					} else if sw.seq == rightNum {
						go func(s *rotaryEncoder) {
							s.updCh <- 1
						}(sw)
					}

					sw.seq = 0
				} else {
					sw.seq = ((sw.seq << 1) + a)
					sw.seq = ((sw.seq << 1) + b)
				}

			}

		}

		time.Sleep(time.Duration(refreshRate) * time.Millisecond)
	}
}
