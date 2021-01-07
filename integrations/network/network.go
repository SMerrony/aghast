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
	"log"
	"sync"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const configFilename = "/network.toml"

// The Network type encapsulates the 'Network' Integration.
// It currently provides the 'HostChecker' deviceType.
type Network struct {
	mqttChan    chan mqtt.MessageT
	networkMu   sync.RWMutex
	HostChecker []hostCheckerT
	stopChans   []chan bool // used for stopping Goroutines
}

// LoadConfig loads and stores the configuration for this Integration
func (n *Network) LoadConfig(confdir string) error {
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Network configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, n)
	if err != nil {
		log.Fatalf("ERROR: Could not load Network config due to %s\n", err.Error())
		return err
	}
	log.Printf("INFO: Network has %d HostCheckers configured\n", len(n.HostChecker))

	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration could supply.
// (This included unconfigured types.)
func (n *Network) ProvidesDeviceTypes() []string {
	return []string{"HostChecker"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (n *Network) Start(evChan chan events.EventT, mq mqtt.MQTT) {

	n.mqttChan = mq.PublishChan

	// HostCheckers
	n.networkMu.RLock()
	for _, dev := range n.HostChecker {
		go n.runHostChecker(dev, evChan)
	}
	n.networkMu.RUnlock()
}

func (n *Network) addStopChan() (ix int) {
	n.networkMu.Lock()
	n.stopChans = append(n.stopChans, make(chan bool))
	ix = len(n.stopChans) - 1
	n.networkMu.Unlock()
	return ix
}

// Stop terminates the Integration and all Goroutines it contains
func (n *Network) Stop() {
	for _, ch := range n.stopChans {
		ch <- true
	}
	log.Println("DEBUG: Network - All Goroutines should have stopped")
}
