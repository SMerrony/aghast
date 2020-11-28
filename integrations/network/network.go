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
	"log"
	"sync"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const configFilename = "/network.toml"

// The Network type encapsulates the 'Network' Integration.
// It currently provides the 'HostChecker' deviceType.
type Network struct {
	hostCheckersMu sync.RWMutex
	hostCheckers   map[string]hostCheckerT
}

// LoadConfig loads and stores the configuration for this Integration
func (n *Network) LoadConfig(confdir string) error {

	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load configuration ", err.Error())
		return err
	}

	n.hostCheckersMu.Lock()
	defer n.hostCheckersMu.Unlock()
	n.hostCheckers = make(map[string]hostCheckerT)

	deviceTypes := conf.Keys()
	log.Printf("DEBUG: Network - Device Types Configured: %v\n", deviceTypes)
	confMap := conf.ToMap()

	for _, devType := range deviceTypes {
		switch devType {
		case "HostChecker":
			mhc := confMap["HostChecker"].(map[string]interface{})
			n.loadHostCheckerConfig(mhc)
		default:
			log.Printf("WARNING: Unknown device type '%s' in Network configuration\n", devType)
		}
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration could supply.
// (This included unconfigured types.)
func (n *Network) ProvidesDeviceTypes() []string {
	return []string{"HostChecker"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (n *Network) Start(evChan chan events.EventT, mqChan chan mqtt.MQTTMessageT) {

	// HostCheckers
	n.hostCheckersMu.RLock()
	for dev := range n.hostCheckers {
		go n.hostChecker(dev, evChan)
	}
	n.hostCheckersMu.RUnlock()
}
