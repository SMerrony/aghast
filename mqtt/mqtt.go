// Copyright ©2020,2021 Steve Merrony

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

// MQTT is not an Integration as it is too central to the core operation of Aghast.

package mqtt

import (
	"fmt"
	"log"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	mqttOutboundQueueLen = 100
	mqttInboundQueueLen  = 100
	// StatusSubtopic is used for sending important system-wide messages
	StatusSubtopic = "/status"
)

// MQTT encapsulates a connection to an MQTT Broker
type MQTT struct {
	PublishChan    chan AghastMsgT
	ThirdPartyChan chan GeneralMsgT
	mutex          sync.RWMutex
	client         mqtt.Client
	options        *mqtt.ClientOptions
	connectHandler mqtt.OnConnectHandler
	connLostHander mqtt.ConnectionLostHandler
	// pubHandler     mqtt.MessageHandler
	subs      map[string][]chan GeneralMsgT
	broker    string
	port      int
	username  string
	password  string
	baseTopic string
}

// AghastMsgT is the type of messages sent via the AGHAST MQTT channels
type AghastMsgT struct {
	Subtopic string
	Qos      byte
	Retained bool
	Payload  interface{}
}

// GeneralMsgT is the type of messages received or sent to third-parties
type GeneralMsgT struct {
	Topic    string
	Qos      byte
	Retained bool
	Payload  interface{}
}

// Disconnect from the MQTT Broker after 100ms
func (m *MQTT) Disconnect() {
	m.client.Disconnect(100)
}

func (m *MQTT) Start(broker string, port int, username string, password string, clientID string, baseTopic string) chan AghastMsgT {
	m.mutex.Lock()
	m.subs = make(map[string][]chan GeneralMsgT)
	m.broker = broker
	m.port = port
	m.username = username
	m.password = password
	m.baseTopic = baseTopic
	m.options = mqtt.NewClientOptions()
	m.options.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	if username != "" {
		m.options.SetUsername(username)
		m.options.SetPassword(password)
	}
	m.options.SetClientID(clientID)

	m.connectHandler = func(client mqtt.Client) {
		log.Println("INFO: AGHAST Connected to MQTT Broker")
	}
	m.options.OnConnect = m.connectHandler

	m.connLostHander = func(client mqtt.Client, err error) {
		log.Printf("WARNING: MQTT Connection lost: %v", err)
	}
	m.options.OnConnectionLost = m.connLostHander

	m.client = mqtt.NewClient(m.options)
	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	m.PublishChan = make(chan AghastMsgT, mqttOutboundQueueLen)
	m.ThirdPartyChan = make(chan GeneralMsgT, mqttOutboundQueueLen)

	m.mutex.Unlock()

	go m.aghastPublish()
	go m.thirdPartyPublish()

	msg := AghastMsgT{
		Subtopic: StatusSubtopic,
		Qos:      0,
		Retained: false,
		Payload:  "Starting",
	}
	m.PublishChan <- msg

	return m.PublishChan

}

// aghastPublish sends messages to any MQTT listeners via the configured Broker
func (m *MQTT) aghastPublish() {
	for {
		msg := <-m.PublishChan
		m.client.Publish(m.baseTopic+msg.Subtopic, msg.Qos, msg.Retained, msg.Payload)
	}
}

// thirdPartyPublish is used to send non-Aghast messages
func (m *MQTT) thirdPartyPublish() {
	for {
		msg := <-m.ThirdPartyChan
		m.client.Publish(msg.Topic, msg.Qos, msg.Retained, msg.Payload)
	}
}

func (m *MQTT) fanOut(topic string) {
	m.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		cMsg := GeneralMsgT{msg.Topic(), msg.Qos(), msg.Retained(), msg.Payload()}
		m.mutex.RLock()
		// log.Printf("DEBUG: mqtt.fanout got a message on %s\n", msg.Topic())
		for _, subChans := range m.subs[topic] {
			subChans <- cMsg
			// log.Println("DEBUG: ... mqtt.fanout forwarding message")
		}
		// log.Println("DEBUG: ... mqtt.fanout done for this message")
		m.mutex.RUnlock()
	})
}

func (m *MQTT) subscribeAndMap(ch chan GeneralMsgT, topic string) {
	m.mutex.RLock()
	_, already := m.subs[topic]
	m.mutex.RUnlock()
	if !already {
		m.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			cMsg := GeneralMsgT{msg.Topic(), msg.Qos(), msg.Retained(), msg.Payload()}
			ch <- cMsg
		})
		go m.fanOut(topic)
	}
	m.mutex.Lock()
	m.subs[topic] = append(m.subs[topic], ch)
	m.mutex.Unlock()
}

// SubscribeToTopic returns a channel which will receive any MQTT messages published to the topic
func (m *MQTT) SubscribeToTopic(topic string) chan GeneralMsgT {
	c := make(chan GeneralMsgT, mqttInboundQueueLen)
	m.subscribeAndMap(c, topic)
	return c
}

// SubscribeToTopicUsingChan uses the provided chan to receive any MQTT messages published to the topic
func (m *MQTT) SubscribeToTopicUsingChan(topic string, c chan GeneralMsgT) {
	m.subscribeAndMap(c, topic)
}

func removeChan(chans []chan GeneralMsgT, i int) []chan GeneralMsgT {
	chans[i] = chans[len(chans)-1]
	return chans[:len(chans)-1]
}

// UnsubscribeFromTopic taked a chan as another parm and use it to correctly
// remove the right subbed chan from the subscription map
func (m *MQTT) UnsubscribeFromTopic(topic string, ch chan GeneralMsgT) {
	m.mutex.RLock()
	subs, found := m.subs[topic]
	m.mutex.RUnlock()
	if !found {
		log.Printf("WARNING: MQTT - UnsubscribeFromTopic called for non-subscribed topic: %s\n", topic)
		return
	}
	for ix, subbedChan := range subs {
		if subbedChan == ch {
			m.mutex.Lock()
			if len(subs) == 1 {
				// this is the only subscriber, so unsubscribe
				m.client.Unsubscribe(topic)
				m.subs[topic] = nil
			} else {
				// there are other subscribers, so just remove from the fan-out list
				m.subs[topic] = removeChan(subs, ix)
			}
			m.mutex.Unlock()
			return
		}
	}
	log.Printf("WARNING: MQTT - UnsubscribeFromTopic did not find matching channel for topic: %s, this should not happen!\n", topic)
}
