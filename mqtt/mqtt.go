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
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const mqttOutboundQueueLen = 100

type mqttT struct {
	// Broker         string
	// Port           int
	// ClientID       string
	client           mqtt.Client
	options          *mqtt.ClientOptions
	connectHandler   mqtt.OnConnectHandler
	connLostHander   mqtt.ConnectionLostHandler
	pubHandler       mqtt.MessageHandler
	subscribedTopics map[string]bool
}

// MQTTMessageT is the type of messages sent via the AGHAST MQTT channels
type MQTTMessageT struct {
	Topic    string
	Qos      byte
	Retained bool
	Payload  interface{}
}

var (
	mq mqttT
	// MQTTPublishChan is used to publish MQTT messages to the Broker
	MQTTPublishChan chan MQTTMessageT
)

func StartMQTT(broker string, port int, clientID string) chan MQTTMessageT {
	mq.subscribedTopics = make(map[string]bool)
	mq.options = mqtt.NewClientOptions()
	mq.options.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	mq.options.SetClientID(clientID)

	mq.connectHandler = func(client mqtt.Client) {
		log.Println("DEBUG: MQTT Connected to Broker")
	}
	mq.options.OnConnect = mq.connectHandler

	mq.connLostHander = func(client mqtt.Client, err error) {
		log.Printf("WARNING: MQTT Connection lost: %v", err)
	}
	mq.options.OnConnectionLost = mq.connLostHander

	mq.pubHandler = func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("DEBUG: Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	}
	mq.options.SetDefaultPublishHandler(mq.pubHandler)

	mq.client = mqtt.NewClient(mq.options)
	if token := mq.client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	MQTTPublishChan = make(chan MQTTMessageT, mqttOutboundQueueLen)
	go PublishViaMQTT()
	msg := MQTTMessageT{
		Topic:    "aghast/status",
		Qos:      0,
		Retained: false,
		Payload:  "Starting",
	}
	MQTTPublishChan <- msg
	// testing...
	// sub(mq.client)
	// go publish(mq.client)

	return MQTTPublishChan

}

// PublishViaMQTT sends messages to any MQTT listeners via the configured Broker
func PublishViaMQTT() {
	for {
		msg := <-MQTTPublishChan
		// log.Printf("DEBUG: PublishViaMQTT got msg to forward for topic: %s, Payload: %v\n", msg.Topic, msg.Payload)
		// // check we already subscribed?
		// if _, subbed := mq.subscribedTopics[msg.Topic]; !subbed {
		// 	token := mq.client.Subscribe(msg.Topic, 1, nil)
		// 	token.Wait()
		// 	log.Printf("DEBUG: MQTT - Subbed to topic: %s\n", msg.Topic)
		// 	mq.subscribedTopics[msg.Topic] = true
		// }
		mq.client.Publish(msg.Topic, msg.Qos, msg.Retained, msg.Payload)
		// token.Wait()
		// log.Println("DEBUG: ... forwarded")
	}
}

// testing...
func publish(client mqtt.Client) {
	num := 10
	for i := 0; i < num; i++ {
		text := fmt.Sprintf("Message %d", i)
		// token := client.Publish("topic/test", 0, false, text)
		// token.Wait()
		msg := MQTTMessageT{
			Topic:    "topic/test",
			Qos:      0,
			Retained: false,
			Payload:  text,
		}
		MQTTPublishChan <- msg
		log.Println("DEBUG: Send msg from Publish")
		time.Sleep(time.Second)
	}
}

// testing...
func sub(client mqtt.Client) {
	topic := "topic/test"
	token := client.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic: %s", topic)
}
