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

package server

import (
	"io/ioutil"
	"log"
	"sort"

	"github.com/SMerrony/aghast/events"
	"github.com/pelletier/go-toml"
)

const (
	automationsSubDir = "/automations"
)

// The Automation type encapsulates Automation
type Automation struct {
	automations []automationT
}

type automationT struct {
	name             string
	description      string
	enabled          bool
	event            events.EventT
	actions          map[string]actionT
	sortedActionKeys []string
}

type actionT struct { // TODO should this be defined elsewhere?
	integration string
	deviceType  string
	deviceLabel string
	control     string
	setting     interface{}
}

func (a *Automation) LoadConfigs(confDir string) error {
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
		newAuto.event.Integration = conf.Get("event.integration").(string)
		newAuto.event.DeviceType = conf.Get("event.deviceType").(string)
		newAuto.event.DeviceName = conf.Get("event.deviceName").(string)
		newAuto.event.EventName = conf.Get("event.eventName").(string)
		confMap := conf.ToMap()
		actsConf := confMap["action"].(map[string]interface{})
		for order, a := range actsConf {
			var act actionT
			details := a.(map[string]interface{})
			act.integration = details["integration"].(string)
			act.deviceType = details["deviceType"].(string)
			act.deviceLabel = details["deviceLabel"].(string)
			act.control = details["control"].(string)
			act.setting = details["setting"] // not cast
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

func StartAutomations(confDir string, evChan chan events.EventT) {

	var autos Automation

	if err := autos.LoadConfigs(confDir); err != nil {
		log.Fatal("ERROR: Cannot proceed with invalid Automations config")
	}

}
