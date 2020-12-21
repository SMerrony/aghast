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
	"time"

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
	bucket, org, token, url string
	client                  influxdb2.Client
	writeAPI                influxAPI.WriteAPI
	loggers                 map[string]loggerT
}

type loggerT struct {
	integration, deviceType string
	deviceName, eventName   string
	dataType                string
}

// LoadConfig loads and stores the configuration for this Integration
func (i *Influx) LoadConfig(confdir string) error {
	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Influx configuration ", err.Error())
		return err
	}
	confMap := conf.ToMap()
	i.bucket = confMap["bucket"].(string)
	i.org = confMap["org"].(string)
	i.token = confMap["token"].(string)
	i.url = confMap["url"].(string)
	i.loggers = make(map[string]loggerT)
	loggersConf := confMap["Logger"].(map[string]interface{})
	for name, l := range loggersConf {
		var logger loggerT
		details := l.(map[string]interface{})
		logger.integration = details["integration"].(string)
		logger.deviceType = details["deviceType"].(string)
		logger.deviceName = details["deviceName"].(string)
		logger.eventName = details["eventName"].(string)
		logger.dataType = details["type"].(string)
		i.loggers[name] = logger
		log.Printf("INFO: InfluxDB logger %s configured as %v\n", name, logger)
	}

	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (i *Influx) ProvidesDeviceTypes() []string {
	return []string{"Logger"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (i *Influx) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	i.client = influxdb2.NewClient(i.url, i.token)
	i.writeAPI = i.client.WriteAPI(i.org, i.bucket)
	for name, l := range i.loggers {
		go i.logger(name, l)
	}
}

func (i *Influx) logger(name string, l loggerT) {
	sid := events.GetSubscriberID(subscribeName)
	ch, err := events.Subscribe(sid, l.integration, l.deviceType, l.deviceName, l.eventName)
	if err != nil {
		log.Printf("WARNING: Influx Integration (logger) could not subscribe to event for %v\n", l)
		return
	}
	// log.Printf("DEBUG: Influx logger starting for %s, %s, subscriber #: %d\n", l.integration, l.eventName, sid)
	for {
		ev := <-ch
		switch l.dataType {
		case "float":
			fl, err := strconv.ParseFloat(ev.Value.(string), 64)
			if err != nil {
				log.Printf("WARNING: Influx logger could not parse float from %v\n", ev.Value.(string))
				continue
			}
			p := influxdb2.NewPoint(name,
				map[string]string{
					"integration": l.integration,
					"deviceType":  l.deviceType,
					"deviceName":  l.deviceName,
				},
				map[string]interface{}{
					l.eventName: fl,
				},
				time.Now())
			i.writeAPI.WritePoint(p)
		case "integer":
			num, err := strconv.Atoi(ev.Value.(string))
			if err != nil {
				log.Printf("WARNING: Influx logger could not parse integer from %v\n", ev.Value.(string))
				continue
			}
			p := influxdb2.NewPoint(name,
				map[string]string{
					"integration": l.integration,
					"deviceType":  l.deviceType,
					"deviceName":  l.deviceName,
				},
				map[string]interface{}{
					l.eventName: num,
				},
				time.Now())
			i.writeAPI.WritePoint(p)
		default:
			// everything else treated as a string
			p := influxdb2.NewPoint(name,
				map[string]string{
					"integration": l.integration,
					"deviceType":  l.deviceType,
					"deviceName":  l.deviceName,
				},
				map[string]interface{}{
					l.eventName: ev.Value,
				},
				time.Now())
			i.writeAPI.WritePoint(p)
		}
		// log.Printf("DEBUG: Influx logger wrote for %s, %s\n", l.integration, l.eventName)
	}
}
