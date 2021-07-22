// Copyright ©2021 Steve Merrony

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

package zigbee2mqtt

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	agconfig "github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	configFilename    = "/zigbee2mqtt.toml"
	subscriberName    = "Z2M"
	mqttPrefix        = "/z2m/"
	changeUpdatePause = 500 * time.Millisecond // wait between operation and requery
)

// The Zigbee2MQTT type encapsulates the Zigbee2MQTT IoT Integration
type Zigbee2MQTT struct {
	conf      confT
	evChan    chan events.EventT
	mqttChan  chan mqtt.GeneralMsgT
	stopChans []chan bool // used for stopping Goroutines
	mq        mqtt.MQTT
	z2mMu     sync.RWMutex
	// lampsByLabel   map[string]int
	socketsByLabel map[string]int
}

// confT fields exported for unmarshalling
type confT struct {
	TopicRoot string
	Socket    []socket
}

type socket struct {
	FriendlyName string
	status       socketStatusT
}

type socketStatusT struct {
	Switch bool
	LQI    int // Link Quality Index (0..255)
}

// LoadConfig loads and stores the configuration for this Integration
func (z *Zigbee2MQTT) LoadConfig(confdir string) error {
	z.z2mMu.Lock()
	defer z.z2mMu.Unlock()
	confBytes, err := agconfig.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read Zigbee2MQTT config due to %s\n", err.Error())
	}
	z.socketsByLabel = make(map[string]int)
	err = toml.Unmarshal(confBytes, &z.conf)
	if err != nil {
		log.Fatalf("ERROR: Could not load Zigbee2MQTT config due to %s\n", err.Error())
	}
	if len(z.conf.Socket) > 0 {
		log.Printf("INFO: Zigbee2MQTT Integration has %d socket(s) configured\n", len(z.conf.Socket))
		for ix, s := range z.conf.Socket {
			z.socketsByLabel[s.FriendlyName] = ix
		}
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (z *Zigbee2MQTT) ProvidesDeviceTypes() []string {
	return []string{"Socket"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (z *Zigbee2MQTT) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	z.evChan = evChan
	z.mqttChan = mq.ThirdPartyChan
	z.mq = mq

	//go z.monitorClients()
	go z.monitorActions() // handle AGHAST Actions
	go z.monitorStates()  // handle responses from devices
	go z.querySockets()   // regularly query sockets

}

func (z *Zigbee2MQTT) addStopChan() (ix int) {
	z.z2mMu.Lock()
	z.stopChans = append(z.stopChans, make(chan bool))
	ix = len(z.stopChans) - 1
	z.z2mMu.Unlock()
	return ix
}

// Stop terminates the Integration and all Goroutines it contains
func (z *Zigbee2MQTT) Stop() {
	for _, ch := range z.stopChans {
		ch <- true
	}
	log.Println("DEBUG: Zigbee2MQTT - All Goroutines should have stopped")
}

// querySockets sends a status request to every socket, every minute
func (z *Zigbee2MQTT) querySockets() {
	sc := z.addStopChan()
	z.z2mMu.RLock()
	stopChan := z.stopChans[sc]
	z.z2mMu.RUnlock()
	everyMinute := time.NewTicker(time.Minute)
	var req mqtt.GeneralMsgT
	for {
		for _, socket := range z.conf.Socket {
			req.Topic = z.conf.TopicRoot + "/" + socket.FriendlyName + "/get"
			req.Payload = "{\"state\": \"\"}"
			req.Qos = 0
			req.Retained = false
			z.mqttChan <- req
		}
		select {
		case <-stopChan:
			return
		case <-everyMinute.C:
			continue
		}
	}
}

func (z *Zigbee2MQTT) monitorStates() {
	sc := z.addStopChan()
	z.z2mMu.RLock()
	stopChan := z.stopChans[sc]
	z.z2mMu.RUnlock()
	updateChan := z.mq.SubscribeToTopic(z.conf.TopicRoot + "/+") // NOT "#", we only want the status updates
	for {
		select {
		case <-stopChan:
			return
		case msg := <-updateChan:
			topicFrName := strings.TrimPrefix(msg.Topic, z.conf.TopicRoot+"/")
			ix, found := z.socketsByLabel[topicFrName]
			if !found {
				log.Printf("INFO: Zigbee2mqtt got unexpected status from " + topicFrName)
				continue
			}
			type Sstatus struct {
				Linkquality uint8
				State       string
			}
			var result Sstatus
			json.Unmarshal([]byte(msg.Payload.([]uint8)), &result)

			switch result.State {
			case "ON":
				z.conf.Socket[ix].status.Switch = true
			case "OFF":
				z.conf.Socket[ix].status.Switch = false
			default:
				log.Printf("WARNING: Zigbee2mqtt got Unexpected state %s", result.State)
			}

			z.conf.Socket[ix].status.LQI = int(result.Linkquality)

		} // select
	} // for
}

func getDeviceName(evName string) string {
	return strings.Split(evName, "/")[events.EvDeviceName]
}

func (z *Zigbee2MQTT) monitorActions() {
	sc := z.addStopChan()
	z.z2mMu.RLock()
	stopChan := z.stopChans[sc]
	z.z2mMu.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, "Zigbee2MQTT"+"/"+events.ActionControlDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: Tuya Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: Zigbee2MQTT Action Monitor got %v\n", ev)
			ix, foundSocket := z.socketsByLabel[getDeviceName(ev.Name)]
			if foundSocket {
				control := strings.Split(ev.Name, "/")[events.EvControl]
				switch control {
				case "state":
					var req mqtt.GeneralMsgT
					req.Topic = z.conf.TopicRoot + "/" + z.conf.Socket[ix].FriendlyName + "/set/state"
					req.Payload = ev.Value.(string)
					req.Qos = 0
					req.Retained = false
					z.mqttChan <- req
				default:
					log.Printf("WARNING: Zigbee2MQTT Action got unknown control <%s>\n", control)
				}
			} else {
				log.Printf("WARNING: Zigbee2MQTT Action monitor got command for unknown unit <%s>\n", getDeviceName(ev.Name))
				z.z2mMu.RUnlock()
				continue
			}
		} // select
	} // for
}
