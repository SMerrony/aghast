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
	Name         string
	Host         string
	Label        string
	Period       int
	Port         int
	alive        bool
	firstCheck   bool
	responseTime time.Duration
}

const mqttPrefix = "aghast/hostchecker/"

func (n *Network) runHostChecker(hc hostCheckerT, evChan chan events.EventT) {
	const (
		netType = "tcp"
		timeout = time.Second * 3
	)

	dest := fmt.Sprintf("%s:%d", hc.Host, hc.Port)
	log.Printf("INFO: Network - HostChecker will monitor host %s - %s\n", dest, hc.Name)
	hc.firstCheck = true
	sc := n.addStopChan()
	n.networkMu.RLock()
	stopChan := n.stopChans[sc]
	n.networkMu.RUnlock()
	ticker := time.NewTicker(time.Duration(hc.Period) * time.Second)
	for {
		before := time.Now()
		_, err := net.DialTimeout(netType, dest, timeout)
		after := time.Now()
		n.networkMu.Lock()
		if err != nil {
			if hc.alive || hc.firstCheck { // has state changed?
				evChan <- events.EventT{
					Integration: "Network",
					DeviceType:  "HostChecker",
					DeviceName:  hc.Name,
					EventName:   "StateChanged",
					Value:       "Unavailable"}
				mqMsg := mqtt.MessageT{
					Topic:    mqttPrefix + hc.Name + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "false",
				}
				n.mqttChan <- mqMsg
			}
			hc.alive = false
		} else {
			if !hc.alive || hc.firstCheck {
				evChan <- events.EventT{
					Integration: "Network",
					DeviceType:  "HostChecker",
					DeviceName:  hc.Name,
					EventName:   "StateChanged",
					Value:       "Available"}
				mqMsg := mqtt.MessageT{
					Topic:    mqttPrefix + hc.Name + "/state",
					Qos:      0,
					Retained: true,
					Payload:  "true",
				}
				n.mqttChan <- mqMsg
			}
			hc.alive = true
			hc.responseTime = after.Sub(before)
			evChan <- events.EventT{
				Integration: "Network",
				DeviceType:  "HostChecker",
				DeviceName:  hc.Name,
				EventName:   "Latency",
				Value:       hc.responseTime}
			n.mqttChan <- mqtt.MessageT{
				Topic:    mqttPrefix + hc.Name + "/latency",
				Qos:      0,
				Retained: true,
				Payload:  fmt.Sprintf("%d", hc.responseTime/time.Millisecond),
			}
		}
		hc.firstCheck = false
		n.networkMu.Unlock()
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			continue
		}
	}
}

// func (n *Network) GetHostNames() (names []string) {
// 	n.networkMu.RLock()
// 	defer n.networkMu.RUnlock()
// 	for n := range n.HostChecker {
// 		names = append(names, n)
// 	}
// 	return names
// }

// func (n *Network) IsHostAlive(dev string) bool {
// 	n.networkMu.RLock()
// 	defer n.networkMu.RUnlock()
// 	return n.HostChecker[dev].alive
// }

// func (n *Network) GetHostResponseTime(dev string) time.Duration {
// 	n.networkMu.RLock()
// 	defer n.networkMu.RUnlock()
// 	return n.HostChecker[dev].responseTime
// }
