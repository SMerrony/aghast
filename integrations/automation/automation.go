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

package automation

import (
	"io/ioutil"
	"log"
	"sort"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	automationsSubDir = "/automation"
	subscribeName     = "AutomationManager"
)

const (
	noEvent eventTypeT = iota
	integrationEvent
	mqttEvent
)

// The Automation type encapsulates Automation
type Automation struct {
	automations []automationT
	evChan      chan events.EventT
	mq          mqtt.MQTT
	stopChans   []chan bool // this could be a map[string]chan bool to stop by name
}

type eventTypeT int

type automationT struct {
	name             string
	description      string
	enabled          bool
	eventType        eventTypeT
	event            events.EventT
	mqttTopic        string
	actions          map[string]actionT
	sortedActionKeys []string
}

type actionT struct { // TODO should this be defined elsewhere?
	integration string
	deviceLabel string
	controls    []string
	settings    []interface{}
}

// LoadConfig loads and stores the configuration for this Integration
func (a *Automation) LoadConfig(confDir string) error {
	confs, err := ioutil.ReadDir(confDir + automationsSubDir)
	if err != nil {
		log.Printf("ERROR: Could not read 'automations' config directory, %v\n", err)
		return err
	}
	for _, conf := range confs {
		log.Printf("DEBUG: Automation manager loading config: %s\n", conf.Name())
		var newAuto automationT
		newAuto.actions = make(map[string]actionT)
		conf, err := toml.LoadFile(confDir + automationsSubDir + "/" + conf.Name())
		if err != nil {
			log.Println("ERROR: Could not load Automation configuration ", err.Error())
			return err
		}
		newAuto.name = conf.Get("name").(string)
		newAuto.description = conf.Get("description").(string)
		newAuto.enabled = conf.Get("enabled").(bool)
		log.Printf("DEBUG: ... %s, %s\n", newAuto.name, newAuto.description)
		if conf.Get("event.integration") != nil {
			newAuto.eventType = integrationEvent
			newAuto.event.Integration = conf.Get("event.integration").(string)
			newAuto.event.DeviceType = conf.Get("event.deviceType").(string)
			newAuto.event.DeviceName = conf.Get("event.deviceName").(string)
			newAuto.event.EventName = conf.Get("event.eventName").(string)
		}
		if conf.Get("event.topic") != nil {
			newAuto.eventType = mqttEvent
			newAuto.mqttTopic = conf.Get("event.topic").(string)
		}
		if newAuto.eventType == noEvent {
			log.Printf("WARNING: Automations - no event specified for %s, ignoring it\n", newAuto.name)
			continue
		}
		confMap := conf.ToMap()
		actsConf := confMap["action"].(map[string]interface{})
		for order, a := range actsConf {
			var act actionT
			details := a.(map[string]interface{})
			act.integration = details["integration"].(string)
			act.deviceLabel = details["deviceLabel"].(string)
			executes := details["execute"].([]interface{})
			for _, ac := range executes {
				cs := ac.(map[string]interface{})
				act.controls = append(act.controls, cs["control"].(string))
				act.settings = append(act.settings, cs["setting"]) // not cast
			}
			newAuto.actions[order] = act
		}
		newAuto.sortedActionKeys = make([]string, 0, len(newAuto.actions))
		for key := range newAuto.actions {
			newAuto.sortedActionKeys = append(newAuto.sortedActionKeys, key)
		}
		sort.Strings(newAuto.sortedActionKeys)
		a.automations = append(a.automations, newAuto)
		log.Printf("DEBUG: ... %v\n", newAuto)
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (a *Automation) ProvidesDeviceTypes() []string {
	return []string{"Automation"}
}

// // StartAutomations launches a Goroutine for each Automation
// func StartAutomations(confDir string, evChan chan events.EventT, mq mqtt.MQTT) {

// 	var autos Automation

// 	autos.evChan = evChan
// 	autos.mq = mq

// 	if err := autos.LoadConfig(confDir); err != nil {
// 		log.Fatal("ERROR: Cannot proceed with invalid Automations config")
// 	}

// 	// for each automation, subscribe to its event
// 	sid := events.GetSubscriberID(subscribeName)
// 	for _, a := range autos.automations {
// 		if a.enabled {
// 			sc := make(chan bool)
// 			switch a.eventType {
// 			case integrationEvent:
// 				go autos.waitForIntegrationEvent(sc, sid, a)
// 			case mqttEvent:
// 				go autos.waitForMqttEvent(sc, a)
// 			}
// 			autos.stopChans = append(autos.stopChans, sc)
// 		}
// 	}

// }

// Start launches a Goroutine for each Automation, LoadConfig() should have been called beforehand.
func (a *Automation) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	a.evChan = evChan
	a.mq = mq

	// for each automation, subscribe to its event
	sid := events.GetSubscriberID(subscribeName)
	for _, auto := range a.automations {
		if auto.enabled {
			sc := make(chan bool)
			switch auto.eventType {
			case integrationEvent:
				go a.waitForIntegrationEvent(sc, sid, auto)
			case mqttEvent:
				go a.waitForMqttEvent(sc, auto)
			}
			a.stopChans = append(a.stopChans, sc)
		}
	}

}
func (a *Automation) waitForIntegrationEvent(stopChan chan bool, sid int, auto automationT) {
	ch, err := events.Subscribe(sid, auto.event.Integration, auto.event.DeviceType, auto.event.DeviceName, auto.event.EventName)
	if err != nil {
		log.Fatalf("ERROR: Automation Manager could not subscribe to event, %v\n", err)
	}
	for {
		log.Printf("DEBUG: Automation Manager waiting for event %s\n", auto.event.EventName)
		select {
		case <-stopChan:
			log.Printf("DEBUG: Automation %s wait stopping", auto.name)
			return
		case <-ch:
			log.Printf("DEBUG: Automation Manager received event %s\n", auto.event.EventName)
			log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
			for _, k := range auto.sortedActionKeys {
				ac := auto.actions[k]
				for i := 0; i < len(ac.controls); i++ {
					a.evChan <- events.EventT{
						Integration: ac.integration,
						DeviceType:  events.ActionControlDeviceType,
						DeviceName:  ac.deviceLabel,
						EventName:   ac.controls[i],
						Value:       ac.settings[i],
					}
					log.Printf("DEBUG: Automation Manager sent event to %s - %s\n", ac.integration, ac.deviceLabel)
					time.Sleep(100 * time.Millisecond) // Don't flood devices with requests
				}
			}
		}
	}
}

func (a *Automation) waitForMqttEvent(stopChan chan bool, auto automationT) {
	mqChan := a.mq.SubscribeToTopic(auto.mqttTopic)
	for {
		select {
		case <-stopChan:
			log.Printf("DEBUG: Automation %s wait stopping", auto.name)
			return
		case <-mqChan:
			log.Printf("DEBUG: Automation Manager received event %s\n", auto.event.EventName)
			log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
			for _, k := range auto.sortedActionKeys {
				ac := auto.actions[k]
				for i := 0; i < len(ac.controls); i++ {
					a.evChan <- events.EventT{
						Integration: ac.integration,
						DeviceType:  events.ActionControlDeviceType,
						DeviceName:  ac.deviceLabel,
						EventName:   ac.controls[i],
						Value:       ac.settings[i],
					}
					log.Printf("DEBUG: Automation Manager sent event to %s - %s\n", ac.integration, ac.deviceLabel)
					time.Sleep(100 * time.Millisecond) // Don't flood devices with requests
				}
			}
		}
	}
}
