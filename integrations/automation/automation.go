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
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	automationsSubDir = "/automation"
	subscribeName     = "AutomationManager"
	mqttPrefix        = "aghast/automation/"
)

// The Automation type encapsulates Automation
type Automation struct {
	confDir           string
	automations       []automationT
	automationsByName map[string]int
	mq                mqtt.MQTT
	stopChans         map[string]chan bool
}

// type eventTypeT int

type automationT struct {
	Name             string
	Description      string
	Enabled          bool
	mqttTopic        string
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
	Topic    string
	controls []string
	settings []interface{}
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
		if conf.Get("Event.Topic") != nil {
			newAuto.mqttTopic = conf.Get("Event.Topic").(string)
		} else {
			log.Printf("WARNING: Automations - no Event specified for %s, ignoring it\n", newAuto.Name)
			continue
		}
		if conf.Get("Condition") != nil {
			newAuto.hasCondition = true
			if conf.Get("Condition.QueryTopic") == nil {
				log.Printf("ERROR: No MQTT Topic found for Condition in %s\n", newAuto.Name)
				continue
			}
			newAuto.condition.QueryTopic = conf.Get("Condition.QueryTopic").(string)
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
// FIXME We should just subscribe to each event, not launch a Goroutine for each one!
func (a *Automation) Start(mq mqtt.MQTT) {
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

func (a *Automation) testCondition(cond conditionT) bool {
	tempMQTT := mqtt.TempConnection(a.mq)
	var respChan chan mqtt.GeneralMsgT
	if cond.ReplyTopic == "" {
		respChan = tempMQTT.SubscribeToTopic(cond.QueryTopic)
	} else {
		respChan = tempMQTT.SubscribeToTopic(cond.ReplyTopic)
	}
	tempMQTT.ThirdPartyChan <- mqtt.GeneralMsgT{
		Topic:    cond.QueryTopic,
		Qos:      0,
		Retained: false,
		Payload:  cond.Payload, // may be empty
	}

	var (
		respAsBool bool
		respAsF64  float64
		respAsI64  int64
		respAsStr  string
	)

	resp := <-respChan // TODO timeout needed

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
			return false // TODO ???
		}
		v, found := jsonMap[cond.Key]
		if !found {
			log.Printf("ERROR: Automation (Condition) - Could find Key in JSON %s\n", resp.Payload.(string))
			return false // TODO ???
		}

		switch v.(type) {
		case bool:
			respAsBool = v.(bool)
		case float64:
			respAsF64 = v.(float64)
		case int64:
			respAsI64 = v.(int64)
		case string:
			respAsStr = v.(string)
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
	mqChan := a.mq.SubscribeToTopic(auto.mqttTopic)
	for {
		select {
		case <-stopChan:
			log.Printf("INFO: Automation %s stopping", auto.Name)
			return
		case <-mqChan:
			// log.Printf("DEBUG: Automation Manager received Event %s\n", auto.Event.Name)
			if !auto.hasCondition || (auto.hasCondition && a.testCondition(auto.condition)) {
				log.Printf("DEBUG: Automation Manager will forward to %d actions\n", len(auto.sortedActionKeys))
				for _, k := range auto.sortedActionKeys {
					ac := auto.actions[k]
					json := "{"
					for i := 0; i < len(ac.controls); i++ {
						if i > 0 {
							json = json + ", "
						}
						json += "\"" + ac.controls[i] + "\":"
						switch ac.settings[i].(type) { // these are the legal TOML types
						case string:
							json += "\"" + ac.settings[i].(string) + "\""
						case bool:
							if ac.settings[i].(bool) {
								json += "true"
							} else {
								json += "false"
							}
						case int:
							json += strconv.Itoa(ac.settings[i].(int))
						case int64:
							json += strconv.Itoa(int(ac.settings[i].(int64)))
						case float64:
							json += fmt.Sprintf("%f", ac.settings[i].(float64))
						default:
							log.Printf("WARNING: Automation %s contains invalid Setting - ignoring\n", auto.Name)
						}
					}
					json += "}"
					a.mq.ThirdPartyChan <- mqtt.GeneralMsgT{
						Topic:    ac.Topic,
						Qos:      0,
						Retained: false,
						Payload:  json,
					}
					log.Printf("DEBUG: Automation Manager sent Event to %s with payload %s\n", ac.Topic, json)
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
