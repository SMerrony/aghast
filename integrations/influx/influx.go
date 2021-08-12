// Copyright ©2020, 2021 Steve Merrony

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
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxAPI "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/pelletier/go-toml"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename = "/influx.toml"
)

// The Influx type encapsulates the Data Logging Integration
type Influx struct {
	Bucket, Org, Token, URL string
	client                  influxdb2.Client
	writeAPI                influxAPI.WriteAPI
	Logger                  []loggerT
	mutex                   sync.RWMutex
	stopChans               []chan bool // used for stopping Goroutines
	mq                      *mqtt.MQTT
}

type loggerT struct {
	Name     string
	Topic    string
	Key      string
	DataType string
}

// LoadConfig loads and stores the configuration for this Integration
func (i *Influx) LoadConfig(confdir string) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()
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

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (i *Influx) Start(mq *mqtt.MQTT) {
	i.mutex.Lock()
	i.mq = mq
	i.client = influxdb2.NewClient(i.URL, i.Token)
	i.writeAPI = i.client.WriteAPI(i.Org, i.Bucket)
	i.mutex.Unlock()
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

func (i *Influx) addStopChan() chan bool {
	newChan := make(chan bool)
	i.mutex.Lock()
	i.stopChans = append(i.stopChans, newChan)
	i.mutex.Unlock()
	return newChan
}

func (i *Influx) logger(l loggerT) {
	ch := i.mq.SubscribeToTopic(l.Topic)
	defer i.mq.UnsubscribeFromTopic(l.Topic)

	stopChan := i.addStopChan()

	log.Printf("INFO: Influx logger starting for %s, optional key: %s\n", l.Topic, l.Key)
	for {
		select {
		case <-stopChan:
			i.writeAPI.Flush()
			return
		case msg := <-ch:
			var value interface{}
			if l.Key == "" {
				value = string(msg.Payload.([]uint8))
			} else {
				jsonMap := make(map[string]interface{})
				err := json.Unmarshal([]byte(msg.Payload.([]uint8)), &jsonMap)
				if err != nil {
					log.Printf("ERROR: Influx - Could not understand JSON %s\n", msg.Payload.(string))
					return
				}
				v, found := jsonMap[l.Key]
				if !found {
					log.Printf("ERROR: Influx - Could find Key in JSON %s\n", msg.Payload.(string))
					return
				}
				value = v
			}
			key := l.Topic
			if l.Key != "" {
				key += "/" + l.Key
			}
			var err error
			switch l.DataType {
			case "float":
				var fl float64
				switch value.(type) {
				case float32:
					fl = float64(value.(float32))
				case float64:
					fl = value.(float64)
				case string:
					fl, err = strconv.ParseFloat(value.(string), 64)
					if err != nil {
						log.Printf("WARNING: Influx logger could not parse float from %v\n", value.(string))
						continue
					}
				}
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"EventName": key,
					},
					map[string]interface{}{
						key: fl,
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			case "integer":
				var num int
				switch value.(type) {
				case int:
					num = value.(int)
				case int32:
					num = int(value.(int32))
				case int64:
					num = int(value.(int64))
				case string:
					num, err = strconv.Atoi(value.(string))
				}
				if err != nil {
					log.Printf("WARNING: Influx logger could not parse integer from %v\n", value.(string))
					continue
				}
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"EventName": key,
					},
					map[string]interface{}{
						key: num,
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			default:
				// everything else treated as a string
				p := influxdb2.NewPoint(l.Name,
					map[string]string{
						"EventName": key,
					},
					map[string]interface{}{
						key: value.(string),
					},
					time.Now())
				i.writeAPI.WritePoint(p)
			}
		}
		// log.Printf("DEBUG: Influx logger wrote for %s, %s\n", l.Integration, l.EventName)
	}
}
