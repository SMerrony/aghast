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

// MQTT is not an Integration as it is too central to the core operation of Aghast.

package mqtt

import (
	"fmt"
	"log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const mqttOutboundQueueLen = 100

// MQTT encapsulates a connection to an MQTT Broker
type MQTT struct {
	PublishChan chan MessageT
	// Broker         string
	// Port           int
	// ClientID       string
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

func (m *MQTT) Start(broker string, port int, clientID string) chan MessageT {
	m.options = mqtt.NewClientOptions()
	m.options.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	m.options.SetClientID(clientID)

	m.connectHandler = func(client mqtt.Client) {
		log.Println("DEBUG: MQTT Connected to Broker")
	}
	m.options.OnConnect = m.connectHandler

	m.connLostHander = func(client mqtt.Client, err error) {
		log.Printf("WARNING: MQTT Connection lost: %v", err)
	}
	m.options.OnConnectionLost = m.connLostHander

	m.pubHandler = func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("DEBUG: Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	}
	m.options.SetDefaultPublishHandler(m.pubHandler)

	m.client = mqtt.NewClient(m.options)
	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	m.PublishChan = make(chan MessageT, mqttOutboundQueueLen)
	go m.publishViaMQTT()

	msg := MessageT{
		Topic:    "aghast/status",
		Qos:      0,
		Retained: false,
		Payload:  "Starting",
	}
	m.PublishChan <- msg
	// testing...
	// sub(m.client)
	// go publish(m.client)

	return m.PublishChan

}

// publishViaMQTT sends messages to any MQTT listeners via the configured Broker
func (m *MQTT) publishViaMQTT() {
	for {
		msg := <-m.PublishChan
		// log.Printf("DEBUG: PublishViaMQTT got msg to forward for topic: %s, Payload: %v\n", msg.Topic, msg.Payload)
		// // check we already subscribed?
		// if _, subbed := m.subscribedTopics[msg.Topic]; !subbed {
		// 	token := m.client.Subscribe(msg.Topic, 1, nil)
		// 	token.Wait()
		// 	log.Printf("DEBUG: MQTT - Subbed to topic: %s\n", msg.Topic)
		// 	m.subscribedTopics[msg.Topic] = true
		// }
		m.client.Publish(msg.Topic, msg.Qos, msg.Retained, msg.Payload)
		// token.Wait()
		// log.Println("DEBUG: ... forwarded")
	}
}

func subscribeToTopic() {

}

// testing...
func sub(client mqtt.Client) {
	topic := "topic/test"
	token := client.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic: %s", topic)
}
