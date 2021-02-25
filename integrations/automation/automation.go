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
	"strconv"
	"strings"
	"time"

	"github.com/SMerrony/aghast/config"
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
	confDir           string
	automations       []automationT
	automationsByName map[string]int
	evChan            chan events.EventT
	mq                mqtt.MQTT
	stopChans         map[string]chan bool
}

type eventTypeT int

type automationT struct {
	Name             string
	Description      string
	Enabled          bool
	eventType        eventTypeT
	Event            events.EventT
	mqttTopic        string
	condition        conditionT
	actions          map[string]actionT
	sortedActionKeys []string
	confFilename     string
}

const (
	isAvailableCond int = iota
	isOnCond
	isValueCond
	isIndexedValueCond
)

type conditionT struct {
	Integration   string
	Name          string
	conditionType int
	Index         int
	IsAvailable   bool
	IsOn          bool
	is            string // comparison operator, one of: "=", "!=", "<", ">", "<=", ">="
	value         interface{}
}

type actionT struct {
	Integration string
	deviceLabel string
	controls    []string
	settings    []interface{}
}

// LoadConfig loads and stores the configuration for this Integration.
// All Automations are loaded, whether they are enabled or not.
func (a *Automation) LoadConfig(confDir string) error {
	a.confDir = confDir
	confs, err := ioutil.ReadDir(confDir + automationsSubDir)
	if err != nil {
		log.Printf("ERROR: Could not read 'automations' config directory, %v\n", err)
		return err
	}
	a.automationsByName = make(map[string]int)
	for _, config := range confs {
		log.Printf("DEBUG: Automation manager loading config: %s\n", config.Name())
		var newAuto automationT
		newAuto.actions = make(map[string]actionT)
		conf, err := toml.LoadFile(confDir + automationsSubDir + "/" + config.Name())
		if err != nil {
			log.Println("ERROR: Could not load Automation configuration ", err.Error())
			return err
		}
		newAuto.Name = conf.Get("Name").(string)
		newAuto.Description = conf.Get("Description").(string)
		newAuto.Enabled = conf.Get("Enabled").(bool)
		newAuto.confFilename = config.Name()
		log.Printf("DEBUG: ... %s, %s\n", newAuto.Name, newAuto.Description)
		if conf.Get("Event.Name") != nil {
			newAuto.eventType = integrationEvent
			newAuto.Event.Name = conf.Get("Event.Name").(string)
		}
		if conf.Get("Event.Topic") != nil {
			newAuto.eventType = mqttEvent
			newAuto.mqttTopic = conf.Get("Event.Topic").(string)
		}
		if newAuto.eventType == noEvent {
			log.Printf("WARNING: Automations - no Event specified for %s, ignoring it\n", newAuto.Name)
			continue
		}
		if conf.Get("Condition") != nil {
			newAuto.condition.Integration = conf.Get("Condition.Integration").(string)
			newAuto.condition.Name = conf.Get("Condition.Name").(string)
			newAuto.condition.conditionType = isValueCond // default
			switch {
			case conf.Get("Condition.IsAvailable") != nil:
				newAuto.condition.IsAvailable = conf.Get("Condition.IsAvailable").(bool)
				newAuto.condition.conditionType = isAvailableCond

			case conf.Get("Condition.Is") != nil:
				newAuto.condition.is = conf.Get("Condition.Is").(string)
				newAuto.condition.value = conf.Get("Condition.Value")

			case conf.Get("Condition.IsOn") != nil:
				newAuto.condition.IsOn = conf.Get("Condition.IsOn").(bool)
				newAuto.condition.conditionType = isOnCond

			case conf.Get("Condition.Index") != nil:
				newAuto.condition.Index = int(conf.Get("Condition.Index").(int64))
				newAuto.condition.conditionType = isIndexedValueCond
			}

		} else {
			// dummy value
			newAuto.condition.Integration = "NONE"
		}
		confMap := conf.ToMap()
		actsConf := confMap["Action"].(map[string]interface{})
		for order, a := range actsConf {
			var act actionT
			details := a.(map[string]interface{})
			act.Integration = details["Integration"].(string)
			act.deviceLabel = details["DeviceLabel"].(string)
			executes := details["Execute"].([]interface{})
			for _, ac := range executes {
				cs := ac.(map[string]interface{})
				act.controls = append(act.controls, cs["Control"].(string))
				act.settings = append(act.settings, cs["Setting"]) // not cast
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
	for ix, au := range a.automations {
		a.automationsByName[au.Name] = ix
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
	// for each automation, subscribe to its Event
	for _, auto := range a.automations {
		if auto.Enabled {
			sc := make(chan bool)
			switch auto.eventType {
			case integrationEvent:
				sid := events.GetSubscriberID(subscribeName)
				go a.waitForIntegrationEvent(sc, sid, auto)
			case mqttEvent:
				go a.waitForMqttEvent(sc, auto)
			}
			a.stopChans[auto.Name] = sc
		} else {
			log.Printf("INFO: Automation %s is not Enabled, will not run\n", auto.Name)
		}
	}
	a.stopChans["mqttMonitor"] = make(chan bool)
	go a.monitorMqtt(a.stopChans["mqttMonitor"])
}

// Stop terminates the Integration and all Goroutines it contains
func (a *Automation) Stop() {
	for _, ch := range a.stopChans {
		ch <- true
		// log.Printf("DEBUG: Asking Automation %s to stop\n", Name)
	}
	log.Println("DEBUG: All Automations should have stopped")
}

func (a *Automation) waitForIntegrationEvent(stopChan chan bool, sid int, auto automationT) {
	ch, err := events.Subscribe(sid, auto.Event.Name)
	if err != nil {
		log.Fatalf("ERROR: Automation Manager could not subscribe to Event, %v\n", err)
	}
	for {
		log.Printf("DEBUG: Automation Manager waiting for Event %s\n", auto.Event.Name)
		select {
		case <-stopChan:
			events.Unsubscribe(sid, auto.Event.Name)
			log.Printf("INFO: Automation %s stopping", auto.Name)
			return
		case <-ch:
			log.Printf("DEBUG: Automation Manager received Event %s\n", auto.Event.Name)
			if auto.condition.Integration == "NONE" || a.testCondition(auto.condition) {
				log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
				for _, k := range auto.sortedActionKeys {
					ac := auto.actions[k]
					for i := 0; i < len(ac.controls); i++ {
						a.evChan <- events.EventT{
							Name:  ac.Integration + "/" + events.ActionControlDeviceType + "/" + ac.deviceLabel + "/" + ac.controls[i],
							Value: ac.settings[i],
						}
						log.Printf("DEBUG: Automation Manager sent Event to %s - %s\n", ac.Integration, ac.deviceLabel)
						time.Sleep(100 * time.Millisecond) // Don't flood devices with requests
					}
				}
			} else {
				log.Println("DEBUG: ... condition not met")
			}
		}
	}
}

func (a *Automation) testCondition(cond conditionT) bool {
	respChan := make(chan interface{})
	switch cond.conditionType {
	case isAvailableCond:
		a.evChan <- events.EventT{
			Name:  cond.Integration + "/" + events.QueryDeviceType + "/" + cond.Name + "/" + events.IsAvailable,
			Value: respChan,
		}
	case isOnCond:
		a.evChan <- events.EventT{
			Name:  cond.Integration + "/" + events.QueryDeviceType + "/" + cond.Name + "/" + events.IsOn,
			Value: respChan,
		}
	case isValueCond:
		a.evChan <- events.EventT{
			Name:  cond.Integration + "/" + events.QueryDeviceType + "/" + cond.Name + "/" + events.FetchLast,
			Value: respChan,
		}
	case isIndexedValueCond:
		a.evChan <- events.EventT{
			Name: cond.Integration + "/" + events.QueryDeviceType + "/" + cond.Name + "/" +
				events.FetchLastIndexed + "/" + strconv.Itoa(cond.Index),
			Value: respChan,
		}
	}
	resp := <-respChan
	log.Printf("DEBUG: Automation manager testCondition got %v\n", resp)
	switch resp.(type) {
	case bool:
		switch cond.conditionType {
		case isAvailableCond:
			return resp.(bool) == cond.IsAvailable
		case isOnCond:
			return resp.(bool) == cond.IsOn
		}
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
			return resp.(int) < int(cond.value.(int64))
		case ">":
			return resp.(int) > int(cond.value.(int64))
		case "=":
			return resp.(int) == int(cond.value.(int64))
		case "!=":
			return resp.(int) != int(cond.value.(int64))
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
			log.Printf("INFO: Automation %s stopping", auto.Name)
			return
		case <-mqChan:
			log.Printf("DEBUG: Automation Manager received Event %s\n", auto.Event.Name)
			if auto.condition.Integration == "NONE" || a.testCondition(auto.condition) {
				log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
				for _, k := range auto.sortedActionKeys {
					ac := auto.actions[k]
					for i := 0; i < len(ac.controls); i++ {
						a.evChan <- events.EventT{
							Name:  ac.Integration + "/" + events.ActionControlDeviceType + "/" + ac.deviceLabel + "/" + ac.controls[i],
							Value: ac.settings[i],
						}
						log.Printf("DEBUG: Automation Manager sent Event to %s - %s\n", ac.Integration, ac.deviceLabel)
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
				log.Printf("WARNING: Automation Manager got invalid MQTT request on topic: %s\n", payload)
				continue
			}
			action := topicSlice[3]
			switch action {
			case "changeEnabled":
				aname := string(msg.Payload.([]uint8))
				// log.Printf("DEBUG: Automation manager got changeEnabled msg %v %s\n", msg, aname)
				newEnabled := !a.automations[a.automationsByName[aname]].Enabled
				a.automations[a.automationsByName[aname]].Enabled = newEnabled
				err := config.ChangeEnabled(a.confDir+automationsSubDir+"/"+a.automations[a.automationsByName[aname]].confFilename, newEnabled)
				if err != nil {
					log.Printf("WARNING: Automation Manager could not rewrite Enabled line in config for: %s\n", a.automations[a.automationsByName[aname]].confFilename)
				}
				if newEnabled {
					sc := make(chan bool)
					switch a.automations[a.automationsByName[aname]].eventType {
					case integrationEvent:
						sid := events.GetSubscriberID(subscribeName)
						go a.waitForIntegrationEvent(sc, sid, a.automations[a.automationsByName[aname]])
					case mqttEvent:
						go a.waitForMqttEvent(sc, a.automations[a.automationsByName[aname]])
					}
					a.stopChans[a.automations[a.automationsByName[aname]].Name] = sc
				} else {
					log.Printf("INFO: Automation Manager Stopping newly disabled Automation %s\n", aname)
					a.stopChans[aname] <- true
					delete(a.stopChans, aname)
					log.Printf("INFO: Automation Manager Stopped newly disabled Automation %s\n", aname)
				}
			case "list":
				type AutoListElementT struct {
					Name, Description string
					Enabled           bool
				}
				var autoList []AutoListElementT
				for _, au := range a.automations {
					le := AutoListElementT{Name: au.Name, Description: au.Description, Enabled: au.Enabled}
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
