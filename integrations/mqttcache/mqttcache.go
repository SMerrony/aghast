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

package mqttcache

import (
	"log"
	"sync"
	"time"

	"github.com/pelletier/go-toml"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename    = "/mqttcache.toml"
	topicPrefix       = "aghast/mqttcache/"
	getTopicPrefix    = topicPrefix + "get/"
	getTopicPrefixLen = len(getTopicPrefix)
)

// MqttCache encapsulates the type of this Integration
type MqttCache struct {
	Cache            []cacheT
	cacheMap         map[string]cacheT
	mutex            sync.RWMutex
	stopChans        []chan bool
	allMsgs, allReqs chan mqtt.GeneralMsgT
	mq               mqtt.MQTT
}

type cacheT struct {
	Topic       string
	RetainSecs  int
	lastMessage mqtt.GeneralMsgT
	lastMsgTime time.Time
}

// LoadConfig func should simply load any config (TOML) files for this Integration
func (m *MqttCache) LoadConfig(confdir string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read MqttCache config due to %s\n", err.Error())
	}
	err = toml.Unmarshal(confBytes, m)
	if err != nil {
		log.Fatalf("ERROR: Could not load MqttCache config due to %s\n", err.Error())
	}
	m.cacheMap = make(map[string]cacheT)
	for _, b := range m.Cache {
		m.cacheMap[b.Topic] = b
	}
	log.Printf("INFO: MqttCache Integration has %d Buffers configured\n", len(m.Cache))
	return nil
}

// Start func begins running the Integration GoRoutines and should return quickly
func (m *MqttCache) Start(mq mqtt.MQTT) {
	m.mq = mq
	// subscribe to all buffer sources and funnel the messages into a single chan
	m.allMsgs = make(chan mqtt.GeneralMsgT)
	m.allReqs = make(chan mqtt.GeneralMsgT)
	for _, cache := range m.Cache {
		m.mq.SubscribeToTopicUsingChan(cache.Topic, m.allMsgs)
		m.mq.SubscribeToTopicUsingChan(getTopicPrefix+cache.Topic, m.allReqs)
	}
	go m.monitorMsgSources()
	go m.monitorRequests()
}

// Stop terminates the Integration and all Goroutines it contains
func (m *MqttCache) Stop() {
	for _, ch := range m.stopChans {
		ch <- true
	}
}

func (m *MqttCache) addStopChan() chan bool {
	newChan := make(chan bool)
	m.mutex.Lock()
	m.stopChans = append(m.stopChans, newChan)
	m.mutex.Unlock()
	return newChan
}

func (m *MqttCache) monitorMsgSources() {
	stopChan := m.addStopChan()
	for {
		select {
		case <-stopChan:
			m.mutex.RLock()
			for _, buff := range m.Cache {
				m.mq.UnsubscribeFromTopic(buff.Topic)
			}
			m.mutex.RUnlock()
			return
		case msg := <-m.allMsgs:
			m.mutex.Lock()
			tmpCache := m.cacheMap[msg.Topic]
			tmpCache.lastMessage = msg
			tmpCache.lastMsgTime = time.Now()
			m.cacheMap[msg.Topic] = tmpCache
			m.mutex.Unlock()
			// log.Printf("DEBUG: mqttcache got data for %s\n", tmpCache.Topic)
		}
	}
}

func (m *MqttCache) monitorRequests() {
	stopChan := m.addStopChan()
	for {
		select {
		case <-stopChan:
			for _, cache := range m.Cache {
				m.mq.UnsubscribeFromTopic(getTopicPrefix + cache.Topic)
			}
			return
		case req := <-m.allReqs:
			// Four possible cases...
			// 1. We have unexpired data
			// 2. We have expired data
			// 3. No data have arrived yet
			// 4. We don't know this topic (not configured)
			reqTopic := req.Topic[getTopicPrefixLen:]
			// log.Printf("DEBUG: mqttcache got request for topic '%s'\n", reqTopic)
			m.mutex.RLock()
			cache, ok := m.cacheMap[reqTopic]
			m.mutex.RUnlock()
			var payload string
			if !ok { // case 4
				payload = "{\"Error\": \"Not configured in mqttcache\"}"
			} else if (cache.lastMsgTime == time.Time{}) { // case 3
				payload = "{\"Error\": \"No data collected yet\"}"
			} else if time.Since(cache.lastMsgTime) > (time.Duration(cache.RetainSecs) * time.Second) {
				payload = "{\"Error\": \"Data expired\"}" // case 2
			} else { // case 1
				payload = string(cache.lastMessage.Payload.([]uint8))
			}
			m.mq.ThirdPartyChan <- mqtt.GeneralMsgT{
				Topic:    topicPrefix + reqTopic,
				Qos:      0,
				Retained: false,
				Payload:  payload,
			}
		}
	}
}
