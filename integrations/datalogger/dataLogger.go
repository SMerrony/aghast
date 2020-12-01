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

package datalogger

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const configFilename = "/datalogger.toml"

// The DataLogger type encapsulates the Data Logging Integration
type DataLogger struct {
	logDir  string
	loggers map[string]loggerT
}

type loggerT struct {
	logFile                 string
	integration, deviceType string
	deviceName, eventName   string
	flushEvery              int
}

// LoadConfig loads and stores the configuration for this Integration
func (d *DataLogger) LoadConfig(confdir string) error {
	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load configuration ", err.Error())
		return err
	}
	d.loggers = make(map[string]loggerT)
	confMap := conf.ToMap()
	d.logDir = confMap["logDir"].(string)
	loggersConf := confMap["Logger"].(map[string]interface{})
	for name, i := range loggersConf {
		var logger loggerT
		details := i.(map[string]interface{})
		logger.logFile = details["logFile"].(string)
		logger.integration = details["integration"].(string)
		logger.deviceType = details["deviceType"].(string)
		// devs := details["devices"].([]interface{})
		// for _, d := range devs {
		// 	logger.devices = append(logger.devices, d.(string))
		// }
		logger.deviceName = details["deviceName"].(string)
		logger.eventName = details["eventName"].(string)
		logger.flushEvery = int(details["flushEvery"].(int64))
		d.loggers[name] = logger
		log.Printf("DEBUG: logger %s configured as %v\n", name, logger)
	}

	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (d *DataLogger) ProvidesDeviceTypes() []string {
	return []string{"CSVlogger"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (d *DataLogger) Start(evChan chan events.EventT, mqChan chan mqtt.MessageT) {
	for _, l := range d.loggers {
		go d.logger(l)
	}
}

func (d *DataLogger) logger(l loggerT) {
	file, err := os.OpenFile(d.logDir+"/"+l.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("WARNING: DataLogger failed to open/create CSV log - %v\n", err)
		return
	}
	csvWriter := csv.NewWriter(file)
	sid := events.GetSubscriberID()
	ch, err := events.Subscribe(sid, l.integration, l.deviceType, l.deviceName, l.eventName)
	if err != nil {
		log.Printf("WARNING: DataLogger Integration could not subscribe to event for %v\n", l)
		return
	}
	idField := l.integration + "/" + l.deviceType + "/" + l.deviceName
	unflushed := 0
	for {
		ev := <-ch
		ts := time.Now().Format(time.RFC3339)
		record := make([]string, 4)
		record[0] = ts
		record[1] = idField
		record[2] = l.eventName
		record[3] = fmt.Sprintf("%v", ev.Value)
		csvWriter.Write(record)
		if unflushed++; unflushed == l.flushEvery {
			csvWriter.Flush()
			unflushed = 0
		}

	}
}
