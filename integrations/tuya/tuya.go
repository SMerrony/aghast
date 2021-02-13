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

package tuya

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	agconfig "github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
	"github.com/tuya/tuya-cloud-sdk-go/api/common"
	"github.com/tuya/tuya-cloud-sdk-go/api/device"
	"github.com/tuya/tuya-cloud-sdk-go/config"
)

const (
	configFilename    = "/tuya.toml"
	subscriberName    = "Tuya"
	mqttPrefix        = "aghast/tuya/"
	changeUpdatePause = 500 * time.Millisecond // wait between operation and requery
)

// The Tuya type encapsulates the Tuya IoT Integration
type Tuya struct {
	conf           confT
	evChan         chan events.EventT
	mqttChan       chan mqtt.MessageT
	stopChans      []chan bool // used for stopping Goroutines
	mq             mqtt.MQTT
	tuyaMu         sync.RWMutex
	lampsByLabel   map[string]int
	socketsByLabel map[string]int
}

// confT fields exported for unmarshalling
type confT struct {
	ApiID      string
	ApiKey     string
	TuyaRegion string
	Lamp       []lamp
	Socket     []socket
}

type lamp struct {
	DeviceID    string
	Label       string
	Dimmable    bool
	Colour      bool
	Temperature bool
	status      lampStatusT
}

type lampStatusT struct {
	SwitchLED     bool
	WorkMode      string
	BrightValueV2 int
	TempValueV2   int
	ColourDataV2  hsvT
}

type hsvT struct {
	H, S, V int
}

type socket struct {
	DeviceID string
	Label    string
	status   socketStatusT
}

type socketStatusT struct {
	Switch1     bool
	Countdown1  float64
	RelayStatus string
	LightMode   string
}

// LoadConfig loads and stores the configuration for this Integration
func (t *Tuya) LoadConfig(confdir string) error {
	t.tuyaMu.Lock()
	defer t.tuyaMu.Unlock()
	confBytes, err := agconfig.PreprocessTOML(confdir, configFilename)
	t.lampsByLabel = make(map[string]int)
	t.socketsByLabel = make(map[string]int)
	if err != nil {
		log.Fatalf("ERROR: Could not read Tuya config due to %s\n", err.Error())
	}
	// conf := confT{}
	err = toml.Unmarshal(confBytes, &t.conf)
	if err != nil {
		log.Fatalf("ERROR: Could not load Tuya config due to %s\n", err.Error())
	}
	log.Printf("DEBUG: Tuya config is... %v\n", t.conf)
	if len(t.conf.Lamp) > 0 {
		log.Printf("INFO: Tuya Integration has %d lamp(s) configured\n", len(t.conf.Lamp))
		for ix, l := range t.conf.Lamp {
			t.lampsByLabel[l.Label] = ix
		}
	}
	if len(t.conf.Socket) > 0 {
		log.Printf("INFO: Tuya Integration has %d socket(s) configured\n", len(t.conf.Socket))
		for ix, s := range t.conf.Socket {
			t.socketsByLabel[s.Label] = ix
		}
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (t *Tuya) ProvidesDeviceTypes() []string {
	return []string{"Lamp", "Socket"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (t *Tuya) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	t.evChan = evChan
	t.mqttChan = mq.PublishChan
	t.mq = mq
	var server string
	switch t.conf.TuyaRegion {
	case "CN":
		server = common.URLCN
	case "EU":
		server = common.URLEU
	case "IN":
		server = common.URLIN
	case "US":
		server = common.URLUS
	default:
		log.Printf("WARNING: Tuya - Unknown Region configured - <%s>\n", t.conf.TuyaRegion)
	}
	config.SetEnv(server, t.conf.ApiID, t.conf.ApiKey)
	//config.SetEnv(server, "", "")

	go t.monitorClients()
	go t.monitorActions()
	go t.monitorLamps()
	go t.monitorSockets()
}

func (t *Tuya) addStopChan() (ix int) {
	t.tuyaMu.Lock()
	t.stopChans = append(t.stopChans, make(chan bool))
	ix = len(t.stopChans) - 1
	t.tuyaMu.Unlock()
	return ix
}

// Stop terminates the Integration and all Goroutines it contains
func (t *Tuya) Stop() {
	for _, ch := range t.stopChans {
		ch <- true
	}
	log.Println("DEBUG: Tuya - All Goroutines should have stopped")
}

// monitorClients waits for client (front-end user) events coming via MQTT and handles them
func (t *Tuya) monitorClients() {

	type clientHSV struct {
		H float64
		S float64
		V float64
		A int
	}

	// config.SetEnv(server, t.conf.ApiID, t.conf.ApiKey)
	clientChan := t.mq.SubscribeToTopic(mqttPrefix + "client/#")
	sc := t.addStopChan()
	t.tuyaMu.RLock()
	stopChan := t.stopChans[sc]
	t.tuyaMu.RUnlock()
	// topic format is aghast/tuya/client/<Label>/<Control>
	for {
		select {
		case <-stopChan:
			return
		case msg := <-clientChan:
			payload := string(msg.Payload.([]uint8))
			topicSlice := strings.Split(msg.Topic, "/")
			t.tuyaMu.RLock()
			var ix int
			var foundLamp, foundSocket bool
			ix, foundLamp = t.lampsByLabel[topicSlice[3]]
			if !foundLamp {
				ix, foundSocket = t.socketsByLabel[topicSlice[3]]
				if !foundSocket {
					log.Printf("WARNING: Tuya front-end monitor got command for unknown unit <%s>\n", topicSlice[3])
					t.tuyaMu.RUnlock()
					continue
				}
			}
			control := topicSlice[4]
			if foundLamp {
				log.Printf("DEBUG: Tuya got control %s for %s with payload %s\n", control, t.conf.Lamp[ix].Label, payload)
				var code, code2 string
				var value, value2 interface{}
				switch control {
				case "switch":
					switch payload {
					case "Off":
						code = "switch_led"
						value = false
					case "White":
						code = "switch_led"
						value = true
						code2 = "work_mode"
						value2 = "white"
					case "Colour":
						code = "switch_led"
						value = true
						code2 = "work_mode"
						value2 = "colour"
					}
				case "switch_led":
					code = "switch_led"
					value = payload == "true" // bool
				case "colour_data_v2":
					code = "colour_data_v2"
					var cd clientHSV
					err := json.Unmarshal([]byte(payload), &cd)
					if err != nil {
						log.Printf("WARNING: Tuya could not unmarshal HSV from client - %s\n", err.Error())
						t.tuyaMu.RUnlock()
						continue
					}
					log.Printf("DEBUG: Tuya - H: %f, S: %f, V: %f\n", cd.H, cd.S, cd.V)
					value = fmt.Sprintf("{\"h\":%d,\"s\":%d,\"v\":%d}", int(cd.H), int(cd.S*1000.0), int(cd.V*1000.0))
					log.Printf("DEBUG: ... encoding to %s\n", value)
				case "bright_value_v2":
					code = "bright_value_v2"
					value, _ = strconv.Atoi(payload)
				case "temp_value_v2":
					code = "temp_value_v2"
					value, _ = strconv.Atoi(payload)
				}
				log.Printf("DEBUG: Tuya sending Code: %s, Value: %v\n", code, value)
				var err error
				if code2 == "" {
					_, err = device.PostDeviceCommand(t.conf.Lamp[ix].DeviceID, []device.Command{{Code: code, Value: value}})
				} else {
					_, err = device.PostDeviceCommand(t.conf.Lamp[ix].DeviceID, []device.Command{{Code: code, Value: value}, {Code: code2, Value: value2}})
				}
				if err != nil {
					log.Printf("WARNING: Tuya Integration got error sending command - %s\n", err.Error())
					t.tuyaMu.RUnlock()
					continue
				}
				t.tuyaMu.RUnlock()
				// force status update so GUI responds nicely
				time.Sleep(changeUpdatePause)
				t.getLampStatus(t.conf.Lamp[ix])
			}
			if foundSocket {
				log.Printf("DEBUG: Tuya got control %s for %s with payload %s\n", control, t.conf.Socket[ix].Label, payload)
				value := false
				if payload == "On" {
					value = true
				}
				_, err := device.PostDeviceCommand(t.conf.Socket[ix].DeviceID, []device.Command{{Code: "switch_1", Value: value}})
				if err != nil {
					log.Printf("WARNING: Tuya Integration got error sending command - %s\n", err.Error())
					t.tuyaMu.RUnlock()
					continue
				}
				t.tuyaMu.RUnlock()
				// force status update so GUI responds nicely
				time.Sleep(changeUpdatePause)
				t.getSocketStatus(t.conf.Socket[ix])
			}
		}
	}
}

func (t *Tuya) getLampStatus(l lamp) {
	status, err := device.GetDeviceStatus(l.DeviceID)
	if err != nil {
		log.Printf("WARNING: Tuya GetDeviceStatus failed with %s\n", err.Error())
	} else {
		// log.Printf("DEBUG: Tuya device status response Code: %d, Message: %s, Success: %v\n", status.Code, status.Msg, status.Success)
		if status.Success {
			var currentStatus lampStatusT
			for _, r := range status.Result {
				// log.Printf("DEBUG: ... Code: %s, Value: %v\n", r.Code, r.Value)
				switch r.Code {
				case "switch_led":
					currentStatus.SwitchLED = r.Value.(bool)
				case "work_mode":
					currentStatus.WorkMode = r.Value.(string)
				case "bright_value_v2":
					currentStatus.BrightValueV2 = int(r.Value.(float64))
				case "temp_value_v2":
					currentStatus.TempValueV2 = int(r.Value.(float64))
				case "colour_data_v2":
					err := json.Unmarshal([]byte(r.Value.(string)), &currentStatus.ColourDataV2)
					if err != nil {
						log.Printf("WARNING: Tuya could not unmarshal HSV data from map, %s\n", err.Error())
					}
				}
			}
			t.tuyaMu.Lock()
			l.status = currentStatus
			t.tuyaMu.Unlock()
			// log.Printf("DEBUG: ... current Status: %v\n", currentStatus)
			payload, err := json.Marshal(currentStatus)
			if err != nil {
				log.Fatalf("ERROR: Tuya could not marshal status info - %s\n", err.Error())
			}
			// log.Println("DEBUG: Tuya - sending MQTT update...")
			t.mqttChan <- mqtt.MessageT{
				Topic:    mqttPrefix + l.Label + "/status",
				Qos:      0,
				Retained: false,
				Payload:  payload,
			}
		}
	}
}

// monitorLamps
func (t *Tuya) monitorLamps() {
	sc := t.addStopChan()
	t.tuyaMu.RLock()
	stopChan := t.stopChans[sc]
	t.tuyaMu.RUnlock()
	everyMinute := time.NewTicker(time.Minute)
	for {
		for _, lamp := range t.conf.Lamp {
			t.getLampStatus(lamp)
		}
		select {
		case <-stopChan:
			return
		case <-everyMinute.C:
			continue
		}
	}
}

func (t *Tuya) getSocketStatus(sock socket) {
	status, err := device.GetDeviceStatus(sock.DeviceID)
	if err != nil {
		log.Printf("WARNING: Tuya GetDeviceStatus failed with %s\n", err.Error())
	} else {
		// log.Printf("DEBUG: Tuya device status response Code: %d, Message: %s, Success: %v\n", status.Code, status.Msg, status.Success)
		if status.Success {
			var currentStatus socketStatusT
			for _, r := range status.Result {
				// log.Printf("DEBUG: ... Code: %s, Value: %v\n", r.Code, r.Value)
				switch r.Code {
				case "switch_1":
					currentStatus.Switch1 = r.Value.(bool)
				case "countdown_1":
					currentStatus.Countdown1 = r.Value.(float64)
				case "relay_status":
					currentStatus.RelayStatus = r.Value.(string)
				case "light_mode":
					currentStatus.LightMode = r.Value.(string)
				}
			}
			t.tuyaMu.Lock()
			sock.status = currentStatus
			t.tuyaMu.Unlock()
			// log.Printf("DEBUG: ... current Status: %v\n", currentStatus)
			payload, err := json.Marshal(currentStatus)
			if err != nil {
				log.Fatalf("ERROR: Tuya could not marshal status info - %s\n", err.Error())
			}
			// log.Println("DEBUG: Tuya - sending MQTT update...")
			t.mqttChan <- mqtt.MessageT{
				Topic:    mqttPrefix + sock.Label + "/status",
				Qos:      0,
				Retained: false,
				Payload:  payload,
			}
		}
	}
}

// monitorSockets
func (t *Tuya) monitorSockets() {
	sc := t.addStopChan()
	t.tuyaMu.RLock()
	stopChan := t.stopChans[sc]
	t.tuyaMu.RUnlock()
	everyMinute := time.NewTicker(time.Minute)
	for {
		for _, socket := range t.conf.Socket {
			t.getSocketStatus(socket)
		}
		select {
		case <-stopChan:
			return
		case <-everyMinute.C:
			continue
		}
	}
}

func getDeviceName(evName string) string {
	return strings.Split(evName, "/")[events.EvDeviceName]
}

// monitorActions listens for Control Actions from Automations and performs them
func (t *Tuya) monitorActions() {
	sc := t.addStopChan()
	t.tuyaMu.RLock()
	stopChan := t.stopChans[sc]
	t.tuyaMu.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, "Tuya"+"/"+events.ActionControlDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: Tuya Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: Tuya Action Monitor got %v\n", ev)
			var ix int
			var foundLamp, foundSocket bool
			ix, foundLamp = t.lampsByLabel[getDeviceName(ev.Name)]
			if !foundLamp {
				ix, foundSocket = t.socketsByLabel[getDeviceName(ev.Name)]
			}
			switch {
			case foundLamp:
				log.Println("WARNING: Tuya Integration does not yet support Lamp Automation Actions")
			case foundSocket:
				control := strings.Split(ev.Name, "/")[events.EvControl]
				switch control {
				case "power":
					value := false
					if ev.Value.(string) == "on" {
						value = true
					}
					_, err := device.PostDeviceCommand(t.conf.Socket[ix].DeviceID, []device.Command{{Code: "switch_1", Value: value}})
					if err != nil {
						log.Printf("WARNING: Tuya Integration got error sending command - %s\n", err.Error())
						t.tuyaMu.RUnlock()
						continue
					}
				default:
					log.Printf("WARNING: Tuya Action got unknown control <%s>\n", control)
				}
			default:
				log.Printf("WARNING: Tuya Action monitor got command for unknown unit <%s>\n", getDeviceName(ev.Name))
				t.tuyaMu.RUnlock()
				continue
			}

		}
	}
}
