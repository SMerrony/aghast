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

package influx

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxAPI "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/influx.toml"
	subscribeName  = "Influx"
)

// The Influx type encapsulates the Data Logging Integration
type Influx struct {
	Bucket, Org, Token, URL string
	client                  influxdb2.Client
	writeAPI                influxAPI.WriteAPI
	Logger                  []loggerT
	influxMu                sync.RWMutex
	stopChans               []chan bool // used for stopping Goroutines
}

type loggerT struct {
	Name                    string
	Integration, DeviceType string
	DeviceName, EventName   string
	DataType                string
}

// LoadConfig loads and stores the configuration for this Integration
func (i *Influx) LoadConfig(confdir string) error {
	i.influxMu.Lock()
	defer i.influxMu.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Influx configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, i)
	if err != nil {
		log.Fatalf("ERROR: Could not load Influx config due to %s\n", err.Error())
		return err
	}
	log.Printf("INFO: Influx has %d loggers\n", len(i.Logger))
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (i *Influx) ProvidesDeviceTypes() []string {
	return []string{"Logger"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (i *Influx) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	i.influxMu.Lock()
	i.client = influxdb2.NewClient(i.URL, i.Token)
	i.writeAPI = i.client.WriteAPI(i.Org, i.Bucket)
	i.influxMu.Unlock()
	for _, l := range i.Logger {
		go i.logger(l)
	}
}

// Stop terminates the Integration and all Goroutines it contains
func (i *Influx) Stop() {
	for _, ch := range i.stopChans {
		ch <- true
	}
	log.Println("DEBUG: Influx - All Goroutines should have stopped")
}

func (i *Influx) addStopChan() (ix int) {
	i.influxMu.Lock()
	i.stopChans = append(i.stopChans, make(chan bool))
	ix = len(i.stopChans) - 1
	i.influxMu.Unlock()
	return ix
}

func (i *Influx) logger(l loggerT) {
	sid := events.GetSubscriberID(subscribeName)
	ch, err := events.Subscribe(sid, l.Integration, l.DeviceType, l.DeviceName, l.EventName)
	if err != nil {
		log.Printf("WARNING: Influx Integration (logger) could not subscribe to event for %v\n", l)
		return
	}
	sc := i.addStopChan()
	i.influxMu.RLock()
	stopChan := i.stopChans[sc]
	i.influxMu.RUnlock()
	log.Printf("DEBUG: Influx logger starting for %s, %s, %s, subscriber #: %d\n", l.Integration, l.DeviceName, l.EventName, sid)
	for {
		select {
		case <-stopChan:
			i.writeAPI.Flush()
			return
		case ev := <-ch:
			switch l.DataType {
			case "float":
				fl, err := strconv.ParseFloat(ev.Value.(string), 64)
				if err != nil {
					log.Printf("WARNING: Influx logger could not parse float from %v\n", ev.Value.(string))
					continue
				}
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"Integration": l.Integration,
						"DeviceType":  l.DeviceType,
						"DeviceName":  l.DeviceName,
					},
					map[string]interface{}{
						l.EventName: fl,
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			case "integer":
				num, err := strconv.Atoi(ev.Value.(string))
				if err != nil {
					log.Printf("WARNING: Influx logger could not parse integer from %v\n", ev.Value.(string))
					continue
				}
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"Integration": l.Integration,
						"DeviceType":  l.DeviceType,
						"DeviceName":  l.DeviceName,
					},
					map[string]interface{}{
						l.EventName: num,
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			default:
				// everything else treated as a string
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"Integration": l.Integration,
						"DeviceType":  l.DeviceType,
						"DeviceName":  l.DeviceName,
					},
					map[string]interface{}{
						l.EventName: ev.Value,
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			}
		}
		// log.Printf("DEBUG: Influx logger wrote for %s, %s\n", l.Integration, l.EventName)
	}
}
