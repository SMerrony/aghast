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

package tuya

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	agconfig "github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
	"github.com/tuya/tuya-cloud-sdk-go/api/common"
	"github.com/tuya/tuya-cloud-sdk-go/api/device"
	"github.com/tuya/tuya-cloud-sdk-go/config"
)

const (
	configFilename = "/tuya.toml"
	mqttPrefix     = "aghast/tuya/"
)

// The Tuya type encapsulates the Tuya IoT Integration
type Tuya struct {
	conf         confT
	evChan       chan events.EventT
	mqttChan     chan mqtt.MessageT
	mq           mqtt.MQTT
	lampsByLabel map[string]int
}

// confT fields exported for unmarshalling
type confT struct {
	ApiID      string
	ApiKey     string
	TuyaRegion string
	Lamp       []lamp
}

type lamp struct {
	DeviceID    string
	Label       string
	Dimmable    bool
	Colour      bool
	Temperature bool
}

// LoadConfig loads and stores the configuration for this Integration
func (t *Tuya) LoadConfig(confdir string) error {
	confBytes, err := agconfig.PreprocessTOML(confdir, configFilename)
	t.lampsByLabel = make(map[string]int)
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
		log.Printf("INFO: Tuya Integration has %d lamps configured\n", len(t.conf.Lamp))
		for ix, l := range t.conf.Lamp {
			t.lampsByLabel[l.Label] = ix
		}
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (t *Tuya) ProvidesDeviceTypes() []string {
	return []string{"Lamp"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (t *Tuya) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	t.evChan = evChan
	// d.mqttChan = mq.PublishChan
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
	// topic format is aghast/tuya/client/<Label>/<Control>
	for {
		msg := <-clientChan
		payload := string(msg.Payload.([]uint8))
		topicSlice := strings.Split(msg.Topic, "/")
		ix, found := t.lampsByLabel[topicSlice[3]]
		if !found {
			log.Printf("WARNING: Tuya front-end monitor got command for unknown unit <%s>\n", topicSlice[3])
			continue
		}
		control := topicSlice[4]

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
			continue
		}
	}
}
