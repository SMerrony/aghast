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

package mqttsender

import (
	"log"
	"sync"
	"time"

	"github.com/pelletier/go-toml"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename = "/mqttsender.toml"
)

// MqttSender encapsulates the type of this Integration
type MqttSender struct {
	Sender    []senderT
	mutex     sync.RWMutex
	stopChans []chan bool
	mq        mqtt.MQTT
}

type senderT struct {
	Topic    string
	Payload  string
	Interval string
	Period   int
	// periodSecs is calculated from the user-provided config
	periodSecs int
}

// LoadConfig func should simply load any config (TOML) files for this Integration
func (m *MqttSender) LoadConfig(confdir string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read MqttSender config due to %s\n", err.Error())
	}
	err = toml.Unmarshal(confBytes, m)
	if err != nil {
		log.Fatalf("ERROR: Could not load MqttSender config due to %s\n", err.Error())
	}
	for i, _ := range m.Sender {
		switch m.Sender[i].Interval {
		case "Seconds":
			m.Sender[i].periodSecs = m.Sender[i].Period
		case "Minutes":
			m.Sender[i].periodSecs = m.Sender[i].Period * 60
		case "Hours":
			m.Sender[i].periodSecs = m.Sender[i].Period * 3600
		case "Days":
			m.Sender[i].periodSecs = m.Sender[i].Period * (3600 * 24)
		default:
			log.Fatalf("ERROR: Could not load MqttSender config due to unknown Interval: %s\n", m.Sender[i].Interval)
		}

	}
	log.Printf("INFO: MqttSender Integration has %d Senders configured\n", len(m.Sender))
	return nil
}

// Start func begins running the Integration GoRoutines and should return quickly
func (m *MqttSender) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	m.mq = mq
	go m.sender()
}

// Stop terminates the Integration and all Goroutines it contains
func (m *MqttSender) Stop() {
	for _, ch := range m.stopChans {
		ch <- true
	}
}

func (m *MqttSender) addStopChan() chan bool {
	newChan := make(chan bool)
	m.mutex.Lock()
	m.stopChans = append(m.stopChans, newChan)
	m.mutex.Unlock()
	return newChan
}

func (m *MqttSender) sender() {
	stopChan := m.addStopChan()
	secs := time.NewTicker(time.Second)
	tock := 0
	for {
		select {
		case <-stopChan:
			return
		case <-secs.C:
			tock++
			// we could add more data structures (indices) to make this a little more efficient
			// but I doubt there's any benefit unless there are thousands of Senders
			for _, s := range m.Sender {
				if tock%s.periodSecs == 0 {
					m.mq.ThirdPartyChan <- mqtt.GeneralMsgT{
						Topic:    s.Topic,
						Qos:      0,
						Retained: false,
						Payload:  s.Payload,
					}
				}
			}
		}
	}
}
