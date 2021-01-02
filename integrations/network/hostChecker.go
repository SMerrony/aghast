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

package network

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
)

type hostCheckerT struct {
	Host  string
	Label string
	// Name         string
	Period       int
	Port         int
	alive        bool
	firstCheck   bool
	responseTime time.Duration
}

const mqttPrefix = "aghast/hostchecker/"

func (n *Network) loadHostCheckerConfig(mhc map[string]interface{}) {
	for name, i := range mhc {
		var hc hostCheckerT
		// hc.Name = name
		details := i.(map[string]interface{})
		hc.Host = details["host"].(string)
		hc.Label = details["label"].(string)
		hc.Period = int(details["period"].(int64))
		hc.Port = int(details["port"].(int64))
		hc.firstCheck = true
		n.hostCheckers[name] = hc
		log.Printf("DEBUG: host: %s\t Label: %s\t Port: %d\n", hc.Host, hc.Label, hc.Port)
	}
}

func (n *Network) hostChecker(dev string, evChan chan events.EventT) {
	const (
		netType = "tcp"
		timeout = time.Second * 3
	)
	n.hostCheckersMu.RLock()
	hcConf := n.hostCheckers[dev]
	n.hostCheckersMu.RUnlock()

	dest := fmt.Sprintf("%s:%d", hcConf.Host, hcConf.Port)
	log.Printf("INFO: Network.HostChecker will monitor host %s\n", dest)

	for {
		before := time.Now()
		_, err := net.DialTimeout(netType, dest, timeout)
		after := time.Now()
		n.hostCheckersMu.Lock()
		if err != nil {
			if hcConf.alive || hcConf.firstCheck { // has state changed?
				evChan <- events.EventT{
					Integration: "Network",
					DeviceType:  "HostChecker",
					DeviceName:  dev,
					EventName:   "StateChanged",
					Value:       "Unavailable"}
				mqMsg := mqtt.MessageT{
					Topic:    mqttPrefix + dev + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "false",
				}
				n.mqttChan <- mqMsg
			}
			hcConf.alive = false
		} else {
			if !hcConf.alive || hcConf.firstCheck {
				evChan <- events.EventT{
					Integration: "Network",
					DeviceType:  "HostChecker",
					DeviceName:  dev,
					EventName:   "StateChanged",
					Value:       "Available"}
				mqMsg := mqtt.MessageT{
					Topic:    mqttPrefix + dev + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "true",
				}
				n.mqttChan <- mqMsg
			}
			hcConf.alive = true
			hcConf.responseTime = after.Sub(before)
			evChan <- events.EventT{
				Integration: "Network",
				DeviceType:  "HostChecker",
				DeviceName:  dev,
				EventName:   "Latency",
				Value:       hcConf.responseTime}
			n.mqttChan <- mqtt.MessageT{
				Topic:    mqttPrefix + dev + "/latency",
				Qos:      0,
				Retained: true,
				Payload:  fmt.Sprintf("%d", hcConf.responseTime/time.Millisecond),
			}
		}
		hcConf.firstCheck = false
		n.hostCheckersMu.Unlock()
		time.Sleep(time.Duration(hcConf.Period) * time.Second)
	}
}

func (n *Network) GetHostNames() (names []string) {
	n.hostCheckersMu.RLock()
	defer n.hostCheckersMu.RUnlock()
	for n := range n.hostCheckers {
		names = append(names, n)
	}
	return names
}

func (n *Network) IsHostAlive(dev string) bool {
	n.hostCheckersMu.RLock()
	defer n.hostCheckersMu.RUnlock()
	return n.hostCheckers[dev].alive
}

func (n *Network) GetHostResponseTime(dev string) time.Duration {
	n.hostCheckersMu.RLock()
	defer n.hostCheckersMu.RUnlock()
	return n.hostCheckers[dev].responseTime
}
