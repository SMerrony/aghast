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

package mqtt2smtp

import (
	"encoding/json"
	"log"
	"net/smtp"
	"sync"

	"github.com/pelletier/go-toml"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename = "/mqtt2smtp.toml"
	sendTopic      = "aghast/mqtt2smtp/send"
	ackTopic       = "aghast/mqtt2smtp/sent"
)

// Mqtt2smtp encapsulates the type of this Integration
type Mqtt2smtp struct {
	mutex sync.RWMutex
	SmtpHost, SmtpPort,
	SmtpUser, SmtpPassword string
	mq       *mqtt.MQTT
	stopChan chan bool
}

// LoadConfig func should simply load any config (TOML) files for this Integration
func (m *Mqtt2smtp) LoadConfig(confdir string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Fatalf("ERROR: Could not read Mqtt2smtp config due to %s\n", err.Error())
	}
	err = toml.Unmarshal(confBytes, m)
	if err != nil {
		log.Fatalf("ERROR: Could not load Mqtt2smtp config due to %s\n", err.Error())
	}
	return nil
}

// Start func begins running the Integration GoRoutines and should return quickly
func (m *Mqtt2smtp) Start(mq *mqtt.MQTT) {
	m.mq = mq
	go m.sender()
}

// Stop terminates the Integration and all Goroutines it contains
func (m *Mqtt2smtp) Stop() {
	m.stopChan <- true
}

func (m *Mqtt2smtp) sender() {
	m.mutex.Lock()
	m.stopChan = make(chan bool)
	m.mutex.Unlock()
	ch := m.mq.SubscribeToTopic(sendTopic)
	for {
		select {
		case <-m.stopChan:
			m.mq.UnsubscribeFromTopic(sendTopic, ch)
			return
		case msg := <-ch:
			jsonMap := make(map[string]interface{})
			err := json.Unmarshal([]byte(msg.Payload.([]uint8)), &jsonMap)
			if err != nil {
				log.Printf("ERROR: mqtt2smtp - Could not parse JSON %s\n", msg.Payload.([]uint8))
				continue
			}
			dest, found := jsonMap["To"]
			if !found {
				log.Printf("ERROR: mqtt2smtp - no 'To' field for message in %s\n", msg.Payload.([]uint8))
				continue
			}
			subject, found := jsonMap["Subject"]
			if !found {
				log.Printf("ERROR: mqtt2smtp - no 'Subject' field for message in %s\n", msg.Payload.([]uint8))
				continue
			}
			body, found := jsonMap["Message"]
			if !found {
				log.Printf("ERROR: mqtt2smtp - no 'Message' field in message in %s\n", msg.Payload.([]uint8))
				continue
			}
			message := "Subject: " + subject.(string) + "\n\n" + body.(string)
			log.Printf("DEBUG: mqtt2smtp User: %s, Password: %s\n", m.SmtpUser, m.SmtpPassword)
			auth := smtp.PlainAuth("", m.SmtpUser, m.SmtpPassword, m.SmtpHost)
			err = smtp.SendMail(m.SmtpHost+":"+m.SmtpPort, auth, m.SmtpUser, []string{dest.(string)}, []byte(message))
			if err != nil {
				log.Printf("ERROR: Could not send email due to %s\n", err)
				continue
			}
		}
	}
}
