// Copyright ©2020,2021 Steve Merrony

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
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/nathan-osman/go-sunrise"
	"github.com/pelletier/go-toml"
)

const (
	integName      = "Time"
	configFilename = "/time.toml"
	tickerType     = "Ticker"
	tickerDev      = "SystemTicker"
	tomlTimeFmt    = "15:04:05"
)

// N.B. We sometimes use the internal 'alert' below rather than the public 'event' for clarity

// The Time Integration produces time-based events for other Integrations to use.
type Time struct {
	mutex               sync.RWMutex
	mq                  *mqtt.MQTT
	Latitude, Longitude float64
	Alert               []timeEventT            `toml:"Event"`
	alertsByTime        map[string][]timeEventT // indexed by "hh:mm:ss"
	stopChans           []chan bool             // used for stopping Goroutines
}

type timeEventT struct {
	Name       string
	Hhmmss     string `toml:"Time"`
	Daily      string // "Sunrise" or "Sunset"
	OffsetMins int64
}

// LoadConfig is required to satisfy the Integration interface.
func (t *Time) LoadConfig(confdir string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Time configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, t)
	if err != nil {
		log.Fatalf("ERROR: Could not load Time config due to %s\n", err.Error())
		return err
	}
	log.Printf("INFO: Time has %d Event alerts configured %f\n", len(t.Alert), t.Longitude)

	t.alertsByTime = make(map[string][]timeEventT)
	for _, ev := range t.Alert {
		var te timeEventT
		te.Name = ev.Name
		var hhmmss string
		if len(ev.Hhmmss) > 0 {
			hhmmss = ev.Hhmmss
			_, _, _, err := getHhmmssFromString(hhmmss)
			if err != nil {
				log.Fatalf("ERROR: Time Integration could not parse time for event %s  - %v\n", ev.Name, err)
			}
		} else {
			if len(ev.Daily) > 0 {
				// For sunrise/sunset we get the next time and use that for the event
				// Time Integration is reloaded every day to update offsets
				var nextTime time.Time
				offset := time.Minute * time.Duration(ev.OffsetMins)
				sunrise, sunset := sunrise.SunriseSunset(t.Latitude, t.Longitude,
					time.Now().Year(), time.Now().Month(), time.Now().Day())
				// log.Printf("DEBUG: Time - %f, %f, %d / %d / %d\n", t.Latitude, t.Longitude,
				// 	time.Now().Year(), time.Now().Month(), time.Now().Day())
				// log.Printf("DEBUG: Time - Sunrise: %s, Sunset: %s\n", sunrise.Format("15:04:05"), sunset.Format("15:04:05"))
				switch ev.Daily {
				case "Sunrise":
					nextTime = sunrise.Add(offset).Local()
				case "Sunset":
					nextTime = sunset.Add(offset).Local()
				default:
					log.Fatalf("ERROR: Time Integration configuration for %s\n", ev.Name)
				}
				hhmmss = nextTime.Format("15:04:05")
			} else {
				log.Fatalf("ERROR: Time Integration configuration for %s\n", ev.Name)
			}
		}
		te.Hhmmss = hhmmss
		t.alertsByTime[hhmmss] = append(t.alertsByTime[hhmmss], te)
		log.Printf("INFO: Timer Event %s set for %s\n", te.Name, te.Hhmmss)
	}
	return nil
}

func getHhmmssFromString(Hhmmss string) (hh, mm, ss int, e error) {
	t := strings.Split(Hhmmss, ":")
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

// Start any services this Integration provides.
func (t *Time) Start(mq *mqtt.MQTT) {
	t.mq = mq
	go t.tickers()
	go t.timeEvents()
}

func (t *Time) addStopChan() chan bool {
	newChan := make(chan bool)
	t.mutex.Lock()
	t.stopChans = append(t.stopChans, newChan)
	t.mutex.Unlock()
	return newChan
}

// Stop terminates the Integration and all Goroutines it contains
func (t *Time) Stop() {
	for _, ch := range t.stopChans {
		ch <- true
	}
	log.Println("WARNING: Time - All Goroutines are stopping")
}

func (t *Time) timeEvents() {
	stopChan := t.addStopChan()
	secs := time.NewTicker(time.Second)
	for {
		select {
		case <-stopChan:
			return
		case tick := <-secs.C:
			HhmmssNow := tick.Format("15:04:05")
			evs, any := t.alertsByTime[HhmmssNow]
			if any {
				for _, te := range evs {
					t.mq.PublishChan <- mqtt.AghastMsgT{
						Subtopic: "/time/events/" + te.Name,
						Qos:      0,
						Retained: false,
						Payload:  "{\"event\": \"" + te.Name + "\"}",
					}
				}
			}
		}
	}
}

func (t *Time) tickers() {
	lastMinute := time.Now().Minute()
	lastHour := time.Now().Hour()
	lastDay := time.Now().Day()
	stopChan := t.addStopChan()
	secs := time.NewTicker(time.Second)
	for {
		select {
		case <-stopChan:
			return
		case tick := <-secs.C:
			sec := strconv.Itoa(tick.Second())
			t.mq.PublishChan <- mqtt.AghastMsgT{Subtopic: "/time/tickers/seconds", Qos: 0, Retained: false, Payload: "{\"second\": " + sec + "}"}
			// new minute?
			if tick.Minute() != lastMinute {
				minute := strconv.Itoa(tick.Minute())
				t.mq.PublishChan <- mqtt.AghastMsgT{Subtopic: "/time/tickers/minutes", Qos: 0, Retained: false, Payload: "{\"minute\": " + minute + "}"}
				lastMinute = tick.Minute()
				// new hour?
				if tick.Hour() != lastHour {
					hour := strconv.Itoa(tick.Hour())
					t.mq.PublishChan <- mqtt.AghastMsgT{Subtopic: "/time/tickers/hours", Qos: 0, Retained: false, Payload: "{\"hour\": " + hour + "}"}
					lastHour = tick.Hour()
					// new day?
					if tick.Day() != lastDay {
						day := strconv.Itoa(tick.Day())
						t.mq.PublishChan <- mqtt.AghastMsgT{Subtopic: "/time/tickers/days", Qos: 0, Retained: false, Payload: "{\"day\": " + day + "}"}
						lastDay = tick.Day()
					}
				}
			}
		}
	}
}
