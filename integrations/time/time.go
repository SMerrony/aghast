// Copyright ©2020 Steve Merrony

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package time

import (
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	integName  = "Time"
	tickerType = "Ticker"
	tickerDev  = "SystemTicker"
)

// The Time Integration produces time-based events for other Integrations to use.
type Time struct {
}

// LoadConfig is required to satisfy the Integration interface.
func (t *Time) LoadConfig(confdir string) error {
	// no config for Time yet
	return nil
}

// ProvidesDeviceTypes returns a slice of strings naming each Device-type this
// Integration supplies.
func (t *Time) ProvidesDeviceTypes() []string {
	return []string{tickerType}
}

// Start any services this Integration provides.
func (t *Time) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	go tickers(evChan)
}

func tickers(evChan chan events.EventT) {
	lastMinute := time.Now().Minute()
	lastHour := time.Now().Hour()
	lastDay := time.Now().Day()
	secs := time.NewTicker(time.Second)
	for {
		t := <-secs.C
		evChan <- events.EventT{Integration: integName, DeviceType: tickerType, DeviceName: tickerDev, EventName: "Second", Value: t}
		// new minute?
		if t.Minute() != lastMinute {
			evChan <- events.EventT{Integration: integName, DeviceType: tickerType, DeviceName: tickerDev, EventName: "Minute", Value: t}
			lastMinute = t.Minute()
			// new hour?
			if t.Hour() != lastHour {
				evChan <- events.EventT{Integration: integName, DeviceType: tickerType, DeviceName: tickerDev, EventName: "Hour", Value: t}
				lastHour = t.Hour()
				// new day?
				if t.Day() != lastDay {
					evChan <- events.EventT{Integration: integName, DeviceType: tickerType, DeviceName: tickerDev, EventName: "Day", Value: t}
					lastDay = t.Day()
				}
			}
		}
	}
}
