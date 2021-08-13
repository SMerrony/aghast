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

package datalogger

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/datalogger.toml"
	subscribeName  = "Logger"
)

// The DataLogger type encapsulates the Data Logging Integration
type DataLogger struct {
	mutex     sync.RWMutex
	LogDir    string
	Logger    []loggerT
	stopChans []chan bool // used for stopping Goroutines
	mq        *mqtt.MQTT
}

type loggerT struct {
	LogFile    string
	Topic      string
	Key        string
	FlushEvery int
}

// LoadConfig loads and stores the configuration for this Integration
func (d *DataLogger) LoadConfig(confdir string) error {
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load DataLogger configuration ", err.Error())
		return err
	}
	d.mutex.Lock()
	err = toml.Unmarshal(confBytes, d)
	if err != nil {
		log.Fatalf("ERROR: Could not load DataLogger config due to %s\n", err.Error())
		d.mutex.Unlock()
		return err
	}
	log.Printf("INFO: DataLogger has %d loggers %v\n", len(d.Logger), d.Logger)
	d.mutex.Unlock()
	return nil
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (d *DataLogger) Start(mq *mqtt.MQTT) {
	d.mq = mq
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

func (d *DataLogger) addStopChan() chan bool {
	newChan := make(chan bool)
	d.mutex.Lock()
	d.stopChans = append(d.stopChans, newChan)
	d.mutex.Unlock()
	return newChan
}

func (d *DataLogger) logger(l loggerT) {
	d.mutex.RLock()
	log.Printf("INFO: DataLogger starting to log to %s\n", l.LogFile)
	file, err := os.OpenFile(d.LogDir+"/"+l.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("WARNING: DataLogger failed to open/create CSV log - %v\n", err)
		d.mutex.RUnlock()
		return
	}
	csvWriter := csv.NewWriter(file)

	ch := d.mq.SubscribeToTopic(l.Topic)
	defer d.mq.UnsubscribeFromTopic(l.Topic, ch)

	d.mutex.RUnlock()
	unflushed := 0
	stopChan := d.addStopChan()

	for {
		select {
		case <-stopChan:
			csvWriter.Flush()
			return
		case ev := <-ch:
			ts := time.Now().Format(time.RFC3339)
			record := make([]string, 5)
			record[0] = ts
			record[1] = ev.Topic
			if l.Key == "" {
				record[3] = fmt.Sprintf("%v", ev.Payload)
			} else {
				record[2] = l.Key
				jsonMap := make(map[string]interface{})
				err := json.Unmarshal([]byte(ev.Payload.([]uint8)), &jsonMap)
				if err != nil {
					log.Printf("ERROR: DataLogger - Could not understand JSON %s\n", ev.Payload.(string))
					return
				}
				v, found := jsonMap[l.Key]
				if !found {
					log.Printf("ERROR: DataLogger - Could find Key in JSON %s\n", ev.Payload.(string))
					return
				}
				record[3] = fmt.Sprintf("%v", v)
			}
			csvWriter.Write(record)
			d.mutex.RLock()
			if unflushed++; unflushed == l.FlushEvery {
				csvWriter.Flush()
				unflushed = 0
			}
			d.mutex.RUnlock()
		}
	}
}
