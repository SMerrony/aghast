// Copyright 2021 Steve Merrony

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

package pimqttgpio

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename = "/pimqttgpio.toml"
	subscriberName = "PiMqttGpio"
	mqttPrefix     = "aghast/pimqttgpio/"
)

// PiMqttGpio encapsulates the type of this Integration
type PiMqttGpio struct {
	Sensor        []sensorT
	sensorsByName map[string]int
	mutex         sync.RWMutex
	stopChans     []chan bool
	evChan        chan events.EventT
	mq            mqtt.MQTT
}

type sensorT struct {
	Name              string
	TopicPrefix       string
	SensorType        string
	ValueType         string
	IgnoreRogueValues bool // Not yet implemented
	RoundToInteger    bool
	ForwardEvent      bool
	ForwardMQTT       bool
	savedString       string
	savedInteger      int
	savedFloat        float64
}

// LoadConfig func should simply load any config (TOML) files for this Integration
func (p *PiMqttGpio) LoadConfig(confdir string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read PiMqttGpio config due to %s\n", err.Error())
	}
	// conf := confT{}
	err = toml.Unmarshal(confBytes, &p)
	if err != nil {
		log.Fatalf("ERROR: Could not load PiMqttGpio config due to %s\n", err.Error())
	}
	p.sensorsByName = make(map[string]int)
	for i, s := range p.Sensor {
		p.sensorsByName[s.Name] = i
	}
	log.Printf("DEBUG: PiMqttGpio config is... %v\n", p)
	if len(p.Sensor) > 0 {
		log.Printf("INFO: PiMqttGpio Integration has %d Sensors configured\n", len(p.Sensor))
	}
	return nil
}

// Start func begins running the Integration GoRoutines and should return quickly
func (p *PiMqttGpio) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	p.evChan = evChan
	p.mq = mq
	for ix := range p.Sensor {
		go p.monitorSensor(ix)
	}
	go p.monitorQueries()
}

// Stop terminates the Integration and all Goroutines it contains
func (p *PiMqttGpio) Stop() {
	for _, ch := range p.stopChans {
		ch <- true
	}
}

// ProvidesDeviceTypes returns a list of Device Type supported by this Integration
func (p *PiMqttGpio) ProvidesDeviceTypes() []string {
	return []string{"Sensor", "Query"}
}

func (p *PiMqttGpio) addStopChan() (ix int) {
	p.mutex.Lock()
	p.stopChans = append(p.stopChans, make(chan bool))
	ix = len(p.stopChans) - 1
	p.mutex.Unlock()
	return ix
}

func (p *PiMqttGpio) monitorQueries() {
	sc := p.addStopChan()
	p.mutex.RLock()
	stopChan := p.stopChans[sc]
	p.mutex.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, subscriberName+"/"+events.QueryDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: PiMqttGpio Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: PiMqttGpio Query Monitor got %v\n", ev)
			switch strings.Split(ev.Name, "/")[events.EvQueryType] {
			case events.FetchLast:
				var val interface{}
				p.mutex.RLock()
				dev := p.sensorsByName[strings.Split(ev.Name, "/")[events.EvDeviceName]]
				switch p.Sensor[dev].ValueType {
				case "string":
					val = p.Sensor[dev].savedString
				case "integer":
					val = p.Sensor[dev].savedInteger
				case "float":
					val = p.Sensor[dev].savedFloat
				}
				p.mutex.RUnlock()
				ev.Value.(chan interface{}) <- val
			default:
				log.Printf("WARNING: PiMqttGpio received unknown query type %s\n", ev.Name)
			}
		}
	}
}

func (p *PiMqttGpio) monitorSensor(ix int) {
	sc := p.addStopChan()
	p.mutex.RLock()
	stopChan := p.stopChans[sc]
	topic := p.Sensor[ix].TopicPrefix + "/sensor/" + p.Sensor[ix].SensorType
	p.mutex.RUnlock()

	mqChan := p.mq.SubscribeToTopic(topic)
	log.Printf("INFO: PiMqttGpio subscribed to %s\n", topic)
	for {
		select {
		case <-stopChan:
			p.mq.UnsubscribeFromTopic(topic)
			return
		case msg := <-mqChan:
			var evValue, mqttValue interface{}
			payload := string(msg.Payload.([]uint8))
			// log.Printf("DEBUG: PiMqttGpio got message: %s %s\n", msg.Topic, payload)
			// log.Printf("DEBUG: ... expecting type: %s\n", p.Sensor[ix].ValueType)
			switch p.Sensor[ix].ValueType {
			case "string":
				evValue = string(payload)
				p.Sensor[ix].savedString = evValue.(string)
				mqttValue = evValue
			case "integer":
				intVal, err := strconv.ParseInt(payload, 10, 0)
				if err != nil {
					log.Printf("WARNING: PiMqttGpio could not convert value '%s' to integer, ignoring\n", payload)
					continue
				}
				evValue = int(intVal)
				p.Sensor[ix].savedInteger = evValue.(int)
				mqttValue = string(payload)
			case "float":
				floatVal, err := strconv.ParseFloat(payload, 64)
				if err != nil {
					log.Printf("WARNING: PiMqttGpio could not convert value '%s' to float, ignoring\n", payload)
					continue
				}
				if p.Sensor[ix].RoundToInteger {
					evValue = int(math.Round(floatVal))
					p.Sensor[ix].savedInteger = evValue.(int)
					mqttValue = fmt.Sprintf("%d", evValue.(int))
				} else {
					evValue = floatVal
					p.Sensor[ix].savedFloat = floatVal
					mqttValue = string(payload)
				}
			default:
				log.Printf("WARNING: PiMqttGpio unexpected Value Type: %s\n", p.Sensor[ix].ValueType)
			}

			if p.Sensor[ix].ForwardEvent {
				p.evChan <- events.EventT{
					Name:  "PiMqttGpio/Sensor/" + p.Sensor[ix].TopicPrefix + "/" + p.Sensor[ix].SensorType,
					Value: evValue,
				}
			}
			if p.Sensor[ix].ForwardMQTT {
				// log.Println("DEBUG: ... will forward to MQTT")
				p.mq.PublishChan <- mqtt.MessageT{
					Topic:    mqttPrefix + p.Sensor[ix].Name,
					Qos:      0,
					Retained: false,
					Payload:  mqttValue,
				}
			}
		}
	}
}
