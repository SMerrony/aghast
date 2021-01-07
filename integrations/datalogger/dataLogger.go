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
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/datalogger.toml"
	subscribeName  = "Logger"
)

// The DataLogger type encapsulates the Data Logging Integration
type DataLogger struct {
	loggerMu  sync.RWMutex
	LogDir    string
	Logger    []loggerT
	stopChans []chan bool // used for stopping Goroutines
}

type loggerT struct {
	Name, LogFile           string
	Integration, DeviceType string
	DeviceName, EventName   string
	FlushEvery              int
}

// LoadConfig loads and stores the configuration for this Integration
func (d *DataLogger) LoadConfig(confdir string) error {
	d.loggerMu.Lock()
	defer d.loggerMu.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load DataLogger configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, d)
	if err != nil {
		log.Fatalf("ERROR: Could not load DataLogger config due to %s\n", err.Error())
		return err
	}
	log.Printf("INFO: DataLogger has %d loggers\n", len(d.Logger))
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (d *DataLogger) ProvidesDeviceTypes() []string {
	return []string{"CSVlogger"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (d *DataLogger) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	for _, l := range d.Logger {
		go d.logger(l)
	}
}

// Stop terminates the Integration and all Goroutines it contains
func (d *DataLogger) Stop() {
	for _, ch := range d.stopChans {
		ch <- true
	}
	log.Println("DEBUG: DataLogger - All Goroutines should have stopped")
}

func (d *DataLogger) addStopChan() (ix int) {
	d.loggerMu.Lock()
	d.stopChans = append(d.stopChans, make(chan bool))
	ix = len(d.stopChans) - 1
	d.loggerMu.Unlock()
	return ix
}

func (d *DataLogger) logger(l loggerT) {
	d.loggerMu.RLock()
	file, err := os.OpenFile(d.LogDir+"/"+l.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("WARNING: DataLogger failed to open/create CSV log - %v\n", err)
		return
	}
	csvWriter := csv.NewWriter(file)
	sid := events.GetSubscriberID(subscribeName)
	ch, err := events.Subscribe(sid, l.Integration, l.DeviceType, l.DeviceName, l.EventName)
	if err != nil {
		log.Printf("WARNING: DataLogger Integration could not subscribe to event for %v\n", l)
		return
	}
	idRoot := l.Integration + "/" + l.DeviceType
	unflushed := 0
	sc := d.addStopChan()
	stopChan := d.stopChans[sc]
	d.loggerMu.Unlock()

	for {
		select {
		case <-stopChan:
			csvWriter.Flush()
			return
		case ev := <-ch:
			ts := time.Now().Format(time.RFC3339)
			record := make([]string, 5)
			record[0] = ts
			record[1] = idRoot
			record[2] = ev.DeviceName
			record[3] = ev.EventName
			record[4] = fmt.Sprintf("%v", ev.Value)
			csvWriter.Write(record)
			if unflushed++; unflushed == l.FlushEvery {
				csvWriter.Flush()
				unflushed = 0
			}
		}
	}
}
