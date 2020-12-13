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
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	integName      = "Time"
	configFilename = "/time.toml"
	tickerType     = "Ticker"
	tickerDev      = "SystemTicker"
	eventType      = "Events"
	tomlTimeFmt    = "15:04:05"
)

// The Time Integration produces time-based events for other Integrations to use.
type Time struct {
	evChan       chan events.EventT
	alertsByTime map[string][]timeEventT // indexed by "hh:mm:ss"
}

type timeEventT struct {
	name   string
	hhmmss string
}

// LoadConfig is required to satisfy the Integration interface.
func (t *Time) LoadConfig(confdir string) error {
	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load configuration ", err.Error())
		return err

	}
	confMap := conf.ToMap()
	eventsConf := confMap["event"].(map[string]interface{})
	t.alertsByTime = make(map[string][]timeEventT)
	for evName, ev := range eventsConf {
		var te timeEventT
		details := ev.(map[string]interface{})
		te.name = evName
		hhmmss := details["time"].(string)
		_, _, _, err := hhmmssFromString(hhmmss)
		if err != nil {
			log.Fatalf("ERROR: Time Integration could not parse time for event %v\n", err)
		}
		te.hhmmss = hhmmss
		t.alertsByTime[hhmmss] = append(t.alertsByTime[hhmmss], te)
		log.Printf("DEBUG: Timer event %s set for %s\n", te.name, te.hhmmss)
	}
	return nil
}

func hhmmssFromString(hhmmss string) (hh, mm, ss int, e error) {
	t := strings.Split(hhmmss, ":")
	hh, e = strconv.Atoi(t[0])
	if e != nil || hh > 23 {
		return 0, 0, 0, e
	}
	mm, e = strconv.Atoi(t[1])
	if e != nil || mm > 59 {
		return 0, 0, 0, e
	}
	ss, e = strconv.Atoi(t[0])
	if e != nil || ss > 60 {
		return 0, 0, 0, e
	}
	return hh, mm, ss, nil
}

// ProvidesDeviceTypes returns a slice of strings naming each Device-type this
// Integration supplies.
func (t *Time) ProvidesDeviceTypes() []string {
	return []string{tickerType, eventType}
}

// Start any services this Integration provides.
func (t *Time) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	t.evChan = evChan
	go tickers(evChan)
	go t.timeEvents()
}

func (t *Time) timeEvents() {
	secs := time.NewTicker(time.Second)
	for {
		tick := <-secs.C
		hhmmssNow := tick.Format("15:04:05")
		evs, any := t.alertsByTime[hhmmssNow]
		if any {
			for _, te := range evs {
				t.evChan <- events.EventT{
					Integration: integName,
					DeviceType:  eventType,
					DeviceName:  "TimedEvent",
					EventName:   te.name,
					Value:       te.hhmmss, // why not? :-)
				}
			}
		}
	}
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
