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

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	automationsSubDir         = "/automation"
	subscribeName             = "AutomationManager"
	mqttPrefix                = "aghast/automation/"
	conditionQueryTimeoutSecs = 5
)

// The Automation type encapsulates Automation
type Automation struct {
	confDir           string
	automations       []automationT
	automationsByName map[string]int
	mq                *mqtt.MQTT
	stopChans         map[string]chan bool
}

// type eventTypeT int

type automationT struct {
	Name             string
	Description      string
	Enabled          bool
	EventTopic       string
	hasCondition     bool
	condition        conditionT
	actions          map[string]actionT
	sortedActionKeys []string
	confFilename     string
}

type conditionT struct {
	QueryTopic string // MQTT topic for querying
	ReplyTopic string // optional MQTT topic for response
	Payload    string // MQTT payload for query
	Key        string // JSON key of condition value
	Index      int
	is         string // comparison operator, one of: "=", "!=", "<", ">", "<=", ">="
	value      interface{}
}

type actionT struct {
	Topic   string
	Payload string
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
		log.Printf("INFO: Automation manager loading config: %s\n", config.Name())
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
		if !newAuto.Enabled {
			log.Printf("INFO: ... Disabled in configuration")
			continue // ignore disabled automations
		}
		newAuto.confFilename = config.Name()
		// log.Printf("DEBUG: ... %s, %s\n", newAuto.Name, newAuto.Description)
		if conf.Get("EventTopic") != nil {
			newAuto.EventTopic = conf.Get("EventTopic").(string)
		} else {
			log.Printf("WARNING: Automations - no Event Topic specified for %s, ignoring it\n", newAuto.Name)
			continue
		}
		if conf.Get("Condition") != nil {
			newAuto.hasCondition = true
			newAuto.condition.QueryTopic = ""
			if conf.Get("Condition.QueryTopic") != nil {
				newAuto.condition.QueryTopic = conf.Get("Condition.QueryTopic").(string)
			}
			newAuto.condition.ReplyTopic = ""
			if conf.Get("Condition.ReplyTopic") != nil {
				newAuto.condition.ReplyTopic = conf.Get("Condition.ReplyTopic").(string)
			}
			newAuto.condition.Key = ""
			if conf.Get("Condition.Key") != nil {
				newAuto.condition.Key = conf.Get("Condition.Key").(string)
			}
			newAuto.condition.Payload = ""
			if conf.Get("Condition.Payload") != nil {
				newAuto.condition.Payload = conf.Get("Condition.Payload").(string)
			}

			if conf.Get("Condition.Is") == nil {
				log.Printf("ERROR: No Is clause found for Condition in %s\n", newAuto.Name)
				continue
			}
			newAuto.condition.is = conf.Get("Condition.Is").(string)
			newAuto.condition.value = conf.Get("Condition.Value")

		} else {
			newAuto.hasCondition = false
		}
		confMap := conf.ToMap()
		actsConf := confMap["Action"].(map[string]interface{})
		for order, a := range actsConf {
			var act actionT
			details := a.(map[string]interface{})
			act.Topic = details["Topic"].(string)
			act.Payload = details["Payload"].(string)
			newAuto.actions[order] = act
		}
		newAuto.sortedActionKeys = make([]string, 0, len(newAuto.actions))
		for key := range newAuto.actions {
			newAuto.sortedActionKeys = append(newAuto.sortedActionKeys, key)
		}
		sort.Strings(newAuto.sortedActionKeys)
		a.automations = append(a.automations, newAuto)
		// log.Printf("DEBUG: ... %v\n", newAuto)
	}
	for ix, au := range a.automations {
		a.automationsByName[au.Name] = ix
	}
	return nil
}

// Start launches a Goroutine for each Automation, LoadConfig() should have been called beforehand.
func (a *Automation) Start(mq *mqtt.MQTT) {
	a.mq = mq
	a.stopChans = make(map[string]chan bool)
	// for each automation, subscribe to its Event
	for _, auto := range a.automations {
		if auto.Enabled {
			sc := make(chan bool)

			go a.waitForMqttEvent(sc, auto)

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

func (a *Automation) testCondition(cond conditionT, eventPayload interface{}) bool {
	var (
		respChan   chan mqtt.GeneralMsgT
		resp       mqtt.GeneralMsgT
		respAsBool bool
		respAsF64  float64
		respAsI64  int64
		respAsStr  string
	)
	if cond.QueryTopic == "" {
		// there's no new query for this condition, we use the payload from the originating event
		resp.Payload = eventPayload
	} else {
		if cond.ReplyTopic == "" {
			respChan = a.mq.SubscribeToTopic(cond.QueryTopic)
			defer a.mq.UnsubscribeFromTopic(cond.QueryTopic, respChan)
		} else {
			respChan = a.mq.SubscribeToTopic(cond.ReplyTopic)
			defer a.mq.UnsubscribeFromTopic(cond.ReplyTopic, respChan)
		}
		a.mq.ThirdPartyChan <- mqtt.GeneralMsgT{
			Topic:    cond.QueryTopic,
			Qos:      0,
			Retained: false,
			Payload:  cond.Payload, // may be empty
		}

		select {
		case resp = <-respChan:
		case <-time.After(conditionQueryTimeoutSecs * time.Second):
			log.Printf("WARNING: Automation (Condition) - MQTT query timed out on topic %s\n", cond.QueryTopic)
			return false
		}
	}

	// we expect either a simple value, or a JSON response in which case a "Key" should have been specified
	if cond.Key == "" {
		switch cond.value.(type) {
		case bool:
			respAsBool = resp.Payload.(bool)
		case float64:
			respAsF64 = resp.Payload.(float64)
		case int64:
			respAsI64 = resp.Payload.(int64)
		case string:
			respAsStr = resp.Payload.(string)
		}
	} else {
		jsonMap := make(map[string]interface{})
		err := json.Unmarshal([]byte(resp.Payload.([]uint8)), &jsonMap)
		if err != nil {
			log.Printf("ERROR: Automation (Condition) - Could not understand JSON %s\n", resp.Payload.(string))
			return false
		}
		v, found := jsonMap[cond.Key]
		if !found {
			// not an event we are interested in
			return false
		}

		switch v := v.(type) {
		case bool:
			respAsBool = v
		case float64:
			respAsF64 = v
		case int64:
			respAsI64 = v
		case string:
			respAsStr = v
		}
	}

	//log.Printf("DEBUG: Automation manager testCondition got %v\n", resp)
	switch cond.value.(type) {
	case bool:
		return respAsBool == cond.value.(bool)
	case float64:
		switch cond.is {
		case "<":
			return respAsF64 < cond.value.(float64)
		case ">":
			return respAsF64 > cond.value.(float64)
		case "=":
			return respAsF64 == cond.value.(float64)
		case "!=":
			return respAsF64 != cond.value.(float64)
		}
	case int:
		switch cond.is {
		case "<":
			return int(respAsI64) < int(cond.value.(int64))
		case ">":
			return int(respAsI64) > int(cond.value.(int64))
		case "=":
			return int(respAsI64) == int(cond.value.(int64))
		case "!=":
			return int(respAsI64) != int(cond.value.(int64))
		}
	case string:
		switch cond.is {
		case "<":
			return respAsStr < cond.value.(string)
		case ">":
			return respAsStr > cond.value.(string)
		case "=":
			return respAsStr == cond.value.(string)
		case "!=":
			return respAsStr != cond.value.(string)
		}
	default:
		log.Printf("WARNING: Automation Manager testCondition got unexpected data type for: %v\n", resp)
	}
	return false
}

func (a *Automation) waitForMqttEvent(stopChan chan bool, auto automationT) {
	mqChan := a.mq.SubscribeToTopic(auto.EventTopic)
	for {
		select {
		case <-stopChan:
			log.Printf("INFO: Automation %s stopping", auto.Name)
			return
		case eventMsg := <-mqChan:
			// log.Printf("DEBUG: Automation Manager received Event %s\n", auto.Event.Name)
			doit := true
			if auto.hasCondition {
				doit = a.testCondition(auto.condition, eventMsg.Payload)
			}
			if doit {
				log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
				for _, k := range auto.sortedActionKeys {
					ac := auto.actions[k]
					a.mq.ThirdPartyChan <- mqtt.GeneralMsgT{
						Topic:    ac.Topic,
						Qos:      0,
						Retained: false,
						Payload:  ac.Payload,
					}
					log.Printf("DEBUG: Automation Manager sent Event to %s with payload %s\n", ac.Topic, ac.Payload)
				}
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

					go a.waitForMqttEvent(sc, a.automations[a.automationsByName[aname]])

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
				a.mq.PublishChan <- mqtt.AghastMsgT{
					Subtopic: "/automation/list",
					Qos:      0,
					Retained: false,
					Payload:  resp,
				}
			}
		}
	}
}
