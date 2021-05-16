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

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	mqttOutboundQueueLen = 100
	mqttInboundQueueLen  = 100
	// StatusTopic is used for sending important system-wide messages
	StatusTopic = "aghast/status"
)

// MQTT encapsulates a connection to an MQTT Broker
type MQTT struct {
	PublishChan    chan MessageT
	client         mqtt.Client
	options        *mqtt.ClientOptions
	connectHandler mqtt.OnConnectHandler
	connLostHander mqtt.ConnectionLostHandler
	pubHandler     mqtt.MessageHandler
}

// MessageT is the type of messages sent via the AGHAST MQTT channels
type MessageT struct {
	Topic    string
	Qos      byte
	Retained bool
	Payload  interface{}
}

func (m *MQTT) Start(broker string, port int, username string, password string, clientID string) chan MessageT {
	m.options = mqtt.NewClientOptions()
	m.options.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	if username != "" {
		m.options.SetUsername(username)
		m.options.SetPassword(password)
	}
	m.options.SetClientID(clientID)

	m.connectHandler = func(client mqtt.Client) {
		log.Println("DEBUG: MQTT Connected to Broker")
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

	m.PublishChan = make(chan MessageT, mqttOutboundQueueLen)
	go m.publishViaMQTT()

	msg := MessageT{
		Topic:    StatusTopic,
		Qos:      0,
		Retained: false,
		Payload:  "Starting",
	}
	m.PublishChan <- msg

	return m.PublishChan

}

// publishViaMQTT sends messages to any MQTT listeners via the configured Broker
func (m *MQTT) publishViaMQTT() {
	for {
		msg := <-m.PublishChan
		m.client.Publish(msg.Topic, msg.Qos, msg.Retained, msg.Payload)
	}
}

// SubscribeToTopic returns a channel which will receive any MQTT messages published to the topic
func (m *MQTT) SubscribeToTopic(topic string) (c chan MessageT) {
	c = make(chan MessageT, mqttInboundQueueLen)
	m.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		cMsg := MessageT{msg.Topic(), msg.Qos(), msg.Retained(), msg.Payload()}
		c <- cMsg
		// log.Printf("DEBUG: MQTT subscription got topic: %s,  msg: %v\n", msg.Topic(), msg.Payload())
	})
	return c
}

// UnsubscribeFromTopic does what you'd expect
func (m *MQTT) UnsubscribeFromTopic(topic string) {
	m.client.Unsubscribe(topic)
}

// testing...
func sub(client mqtt.Client) {
	topic := "topic/test"
	token := client.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic: %s", topic)
}
