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

package hostchecker

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

// The HostChecker type encapsulates the HostChecker Integration
type HostChecker struct {
	mqttChan       chan mqtt.AghastMsgT
	mutex          sync.RWMutex
	Checker        []hostCheckerT
	checkersByName map[string]int
	stopChans      []chan bool // used for stopping Goroutines
}

type hostCheckerT struct {
	Name         string
	Host         string
	Label        string
	Period       int
	Port         int
	alive        bool
	firstCheck   bool
	responseTime time.Duration
}

const (
	configFilename  = "/hostchecker.toml"
	integrationName = "HostChecker"
	mqttPrefix      = "/hostchecker/"
)

// LoadConfig func should simply load any config (TOML) files for this Integration
func (h *HostChecker) LoadConfig(confdir string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read HostChecker config due to %s\n", err.Error())
	}
	err = toml.Unmarshal(confBytes, h)
	if err != nil {
		log.Fatalf("ERROR: Could not load HostChecker config due to %s\n", err.Error())
	}
	h.checkersByName = make(map[string]int)
	for i, c := range h.Checker {
		h.checkersByName[c.Name] = i
	}
	if len(h.Checker) > 0 {
		log.Printf("INFO: HostChecker Integration has %d checkers configured\n", len(h.Checker))
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration could supply.
// (This included unconfigured types.)
func (h *HostChecker) ProvidesDeviceTypes() []string {
	return []string{integrationName, "Query"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (h *HostChecker) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	h.mqttChan = mq.PublishChan
	for _, dev := range h.Checker {
		go h.runChecker(dev, evChan)
	}
	go h.monitorQueries()
}

func (h *HostChecker) addStopChan() (ix int) {
	h.mutex.Lock()
	h.stopChans = append(h.stopChans, make(chan bool))
	ix = len(h.stopChans) - 1
	h.mutex.Unlock()
	return ix
}

// Stop terminates the Integration and all Goroutines it contains
func (h *HostChecker) Stop() {
	for _, ch := range h.stopChans {
		ch <- true
	}
}

func (h *HostChecker) runChecker(hc hostCheckerT, evChan chan events.EventT) {
	const (
		netType = "tcp"
		timeout = time.Second * 3
	)

	dest := fmt.Sprintf("%s:%d", hc.Host, hc.Port)
	log.Printf("INFO: HostChecker will monitor host %s - %s\n", dest, hc.Name)
	hc.firstCheck = true
	sc := h.addStopChan()
	h.mutex.RLock()
	stopChan := h.stopChans[sc]
	h.mutex.RUnlock()
	ticker := time.NewTicker(time.Duration(hc.Period) * time.Second)
	for {
		before := time.Now()
		_, err := net.DialTimeout(netType, dest, timeout)
		after := time.Now()
		h.mutex.Lock()
		if err != nil {
			if hc.alive || hc.firstCheck { // has state changed?
				mqMsg := mqtt.AghastMsgT{
					Subtopic: mqttPrefix + hc.Name + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "false",
				}
				h.mqttChan <- mqMsg
			}
			hc.alive = false
		} else {
			if !hc.alive || hc.firstCheck {
				mqMsg := mqtt.AghastMsgT{
					Subtopic: mqttPrefix + hc.Name + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "true",
				}
				h.mqttChan <- mqMsg
			}
			hc.alive = true
			hc.responseTime = after.Sub(before)
			h.mqttChan <- mqtt.AghastMsgT{
				Subtopic: mqttPrefix + hc.Name + "/latency",
				Qos:      0,
				Retained: true,
				Payload:  fmt.Sprintf("%d", hc.responseTime/time.Millisecond),
			}
		}
		hc.firstCheck = false
		h.Checker[h.checkersByName[hc.Name]] = hc
		h.mutex.Unlock()
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			continue
		}
	}
}

func (h *HostChecker) monitorQueries() {
	sc := h.addStopChan()
	h.mutex.RLock()
	stopChan := h.stopChans[sc]
	h.mutex.RUnlock()
	sid := events.GetSubscriberID(integrationName)
	// events are HostChecker/Query/CheckerName/<FetchLast | IsAvailable>
	ch, err := events.Subscribe(sid, integrationName+"/"+events.QueryDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: PiMqttGpio Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: HostChecker Query Monitor got %v\n", ev)
			switch strings.Split(ev.Name, "/")[3] {
			case events.IsAvailable:
				var isAlive bool
				h.mutex.RLock()
				isAlive = h.Checker[h.checkersByName[strings.Split(ev.Name, "/")[2]]].alive
				h.mutex.RUnlock()
				// log.Printf("DEBUG: HostChecker Query - %s yields %v\n", ev.Name, h.Checker[h.checkersByName[strings.Split(ev.Name, "/")[2]]])
				if isAlive {
					ev.Value.(chan interface{}) <- true
				} else {
					ev.Value.(chan interface{}) <- false
				}
			default:
				log.Printf("WARNING: HostChecker received unknown query type %s\n", ev.Name)
			}
		}
	}
}
