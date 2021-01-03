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

package server

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/integrations/automation"
	"github.com/SMerrony/aghast/integrations/daikin"
	"github.com/SMerrony/aghast/integrations/datalogger"
	"github.com/SMerrony/aghast/integrations/influx"
	"github.com/SMerrony/aghast/integrations/network"
	"github.com/SMerrony/aghast/integrations/scraper"
	"github.com/SMerrony/aghast/integrations/time"
	"github.com/SMerrony/aghast/integrations/tuya"
	"github.com/SMerrony/aghast/mqtt"
)

// The Integration interface defines the minimal set of methods that an
// Integration must provide
type Integration interface {
	// The LoadConfig func should simply load any config (TOML) files for this Integration
	LoadConfig(string) error

	// The Start func begins running the Integration GoRoutines and should return quickly
	Start(chan events.EventT, mqtt.MQTT)

	// Stop terminates the Integration and all Goroutines it contains
	Stop()

	// ProvidesDeviceType returns a list of Device Type supported by this Integration
	ProvidesDeviceTypes() []string
}

var integs = make(map[string]Integration)

// StartIntegrations asks each enabled Integration to configure itself, then starts them.
func StartIntegrations(conf config.MainConfigT, evChan chan events.EventT, mqtt mqtt.MQTT) {
	for _, i := range conf.Integrations {
		switch i {
		case "automation":
			integs[i] = new(automation.Automation)
		case "daikin":
			integs[i] = new(daikin.Daikin)
		case "datalogger":
			integs[i] = new(datalogger.DataLogger)
		case "influx":
			integs[i] = new(influx.Influx)
		case "network":
			integs[i] = new(network.Network)
		case "scraper":
			integs[i] = new(scraper.Scraper)
		case "time":
			integs[i] = new(time.Time)
		case "tuya":
			integs[i] = new(tuya.Tuya)
		default:
			log.Printf("WARNING: Integration '%s' is not yet handled\n", i)
			continue
		}

		log.Printf("INFO: Integration %s provides %v\n", i, integs[i].ProvidesDeviceTypes())
		if err := integs[i].LoadConfig(conf.ConfigDir); err != nil {
			log.Fatalf("ERROR: %s Integration could not load its configuration", i)
		}
		go integs[i].Start(evChan, mqtt)
	}

	// catch HUP signal to reload Integrations...
	// Only handling Automations for now
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGHUP)
		for {
			<-sigChan
			log.Println("INFO: Got HUP signal to reload Automations")
			i := "automation"
			integs[i].Stop()
			delete(integs, i)
			integs[i] = new(automation.Automation)
			if err := integs[i].LoadConfig(conf.ConfigDir); err != nil {
				log.Fatalf("ERROR: %s Integration could not load its configuration", i)
			}
			go integs[i].Start(evChan, mqtt)
		}
	}()
}
