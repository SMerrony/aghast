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

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/SMerrony/aghast/server"
)

var configFlag = flag.String("configdir", "", "directory containing configuration files")

func main() {
	// check flags
	flag.Parse()
	if *configFlag == "" {
		log.Fatalln("ERROR: You must supply a -configdir")
	}

	// sanity check on config directory
	err := config.CheckMainConfig(*configFlag)
	if err != nil {
		log.Println("ERROR: Main configuration check failed")
		log.Fatalln("ERROR: " + err.Error())
	}

	conf, err := config.LoadMainConfig(*configFlag)
	if err != nil {
		log.Fatalf("ERROR: Failed to load main config file with: %s", err.Error())
	}

	mq := mqtt.MQTT{}
	mqttChan := mq.Start(conf.MqttBroker, conf.MqttPort, conf.MqttUsername, conf.MqttPassword, conf.MqttClientID, conf.MqttBaseTopic)

	server.StartIntegrations(conf, mq)

	mqttChan <- mqtt.AghastMsgT{
		Subtopic: "/status",
		Qos:      0,
		Retained: false,
		Payload:  "Started",
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

}
