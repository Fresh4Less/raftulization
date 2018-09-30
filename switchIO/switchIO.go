// +build !norpio

package switchIO

import (
	"fmt"
	"time"
	"sync"
	"github.com/stianeikeland/go-rpio"
)

var (
	initialized     = false
	refreshRate     = 100 //Milliseconds
	watchedSwitches = []*switchPin{}
	mutex = &sync.Mutex{}
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

//NewSwitchIO factory for a switch object
func NewSwitchIO(pin int) chan int {
	mutex.Lock()
	defer mutex.Unlock()

	if !initialized {
		success := initialize()
		if !success {
			return nil
		}
	}

	s := new(switchPin)

	s.pin = rpio.Pin(pin)

	s.pin.Input()
	s.pin.PullUp()

	s.prevVal = -1
	s.updateCh = make(chan int)

	watchedSwitches = append(watchedSwitches, s)

	return s.updateCh
}

type switchPin struct {
	pin      rpio.Pin
	prevVal  int
	updateCh chan int
}

func watchChanges() {

	defer rpio.Close()

	for true {

		for _, sw := range watchedSwitches {
			read := int(sw.pin.Read())
			if read != sw.prevVal {
				sw.prevVal = read
				go func(s *switchPin) {
					s.updateCh <- read
				}(sw)
			}
		}

		time.Sleep(time.Duration(refreshRate) * time.Millisecond)
	}
}
