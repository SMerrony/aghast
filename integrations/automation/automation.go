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
	"encoding/json"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	automationsSubDir = "/automation"
	subscribeName     = "AutomationManager"
	mqttPrefix        = "aghast/automation/"
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
	stopChans   map[string]chan bool
}

type eventTypeT int

type automationT struct {
	name             string
	description      string
	enabled          bool
	eventType        eventTypeT
	event            events.EventT
	mqttTopic        string
	condition        conditionT
	actions          map[string]actionT
	sortedActionKeys []string
}

type conditionT struct {
	integration string
	name        string
	is          string // comparison operator, one of: "=", "!=", "<", ">", "<=", ">="
	value       interface{}
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
		if conf.Get("condition") != nil {
			newAuto.condition.integration = conf.Get("condition.integration").(string)
			newAuto.condition.name = conf.Get("condition.name").(string)
			newAuto.condition.is = conf.Get("condition.is").(string)
			newAuto.condition.value = conf.Get("condition.value")
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

// Start launches a Goroutine for each Automation, LoadConfig() should have been called beforehand.
func (a *Automation) Start(evChan chan events.EventT, mq mqtt.MQTT) {

	a.evChan = evChan
	a.mq = mq
	a.stopChans = make(map[string]chan bool)

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
			a.stopChans[auto.name] = sc
		} else {
			log.Printf("INFO: Automation %s is not enabled, will not run\n", auto.name)
		}
	}
	a.stopChans["mqttMonitor"] = make(chan bool)
	go a.monitorMqtt(a.stopChans["mqttMonitor"])
}

// Stop terminates the Integration and all Goroutines it contains
func (a *Automation) Stop() {
	for _, ch := range a.stopChans {
		ch <- true
		// log.Printf("DEBUG: Asking Automation %s to stop\n", name)
	}
	log.Println("DEBUG: All Automations should have stopped")
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
			log.Printf("INFO: Automation %s stopping", auto.name)
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

func (a *Automation) testCondition(cond conditionT) bool {
	respChan := make(chan interface{})
	a.evChan <- events.EventT{
		Integration: cond.integration,
		DeviceType:  events.QueryDeviceType,
		DeviceName:  cond.name,
		EventName:   events.FetchLast,
		Value:       respChan,
	}
	resp := <-respChan
	log.Printf("DEBUG: Automation manager testCondition got %v\n", resp)
	switch resp.(type) {
	case float64:
		switch cond.is {
		case "<":
			return resp.(float64) < cond.value.(float64)
		case ">":
			return resp.(float64) > cond.value.(float64)
		case "=":
			return resp.(float64) == cond.value.(float64)
		case "!=":
			return resp.(float64) != cond.value.(float64)
		}
	case int:
		switch cond.is {
		case "<":
			return resp.(int) < cond.value.(int)
		case ">":
			return resp.(int) > cond.value.(int)
		case "=":
			return resp.(int) == cond.value.(int)
		case "!=":
			return resp.(int) != cond.value.(int)
		}
	case string:
		switch cond.is {
		case "<":
			return resp.(string) < cond.value.(string)
		case ">":
			return resp.(string) > cond.value.(string)
		case "=":
			return resp.(string) == cond.value.(string)
		case "!=":
			return resp.(string) != cond.value.(string)
		}
	default:
		log.Printf("WARNING: Automation Manager testCondition got unexpected data type for: %v\n", resp)
	}
	return false
}

func (a *Automation) waitForMqttEvent(stopChan chan bool, auto automationT) {
	mqChan := a.mq.SubscribeToTopic(auto.mqttTopic)
	for {
		select {
		case <-stopChan:
			log.Printf("INFO: Automation %s stopping", auto.name)
			return
		case <-mqChan:
			log.Printf("DEBUG: Automation Manager received event %s\n", auto.event.EventName)
			if a.testCondition(auto.condition) {
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
			} else {
				log.Println("DEBUG: ... condition not met")
			}
		}
	}
}

func (a *Automation) monitorMqtt(stopChan chan bool) {
	reqChan := a.mq.SubscribeToTopic(mqttPrefix + "client/#")
	// topic format is aghast/automation/client/<action>
	for {
		select {
		case <-stopChan:
			return
		case msg := <-reqChan:
			payload := string(msg.Payload.([]uint8))
			topicSlice := strings.Split(msg.Topic, "/")
			if len(topicSlice) < 4 {
				log.Printf("WARNING: Automation manager got invalid MQTT request on topic: %s\n", payload)
				continue
			}
			action := topicSlice[3]
			switch action {
			case "list":
				type AutoListElementT struct {
					Name, Description string
					Enabled           bool
				}
				var autoList []AutoListElementT
				for _, au := range a.automations {
					le := AutoListElementT{Name: au.name, Description: au.description, Enabled: au.enabled}
					autoList = append(autoList, le)
				}
				resp, err := json.Marshal(autoList)
				if err != nil {
					log.Fatalln("ERROR: Automation manager fatal error marshalling data to JSON")
				}
				a.mq.PublishChan <- mqtt.MessageT{
					Topic:    mqttPrefix + "list",
					Qos:      0,
					Retained: false,
					Payload:  resp,
				}
			}
		}
	}
}
