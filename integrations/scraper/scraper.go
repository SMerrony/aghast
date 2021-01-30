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

package scraper

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/gocolly/colly/v2"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/scraper.toml"
	mqttPrefix     = "aghast/scraper/"
	subscriberName = "Scraper"
)

// The Scraper type encapsulates the web scraper Integration.
type Scraper struct {
	mq             mqtt.MQTT
	mutex          sync.RWMutex
	Scrape         []scraperT
	scrapersByName map[string]int
	stopChans      []chan bool // used for stopping Goroutines
}

type scraperT struct {
	Name      string
	URL       string
	Interval  int
	Selector  string
	Attribute string
	Indices   []int
	Subtopics []string
	// Factor    float64
	Suffix       string
	ValueType    string // One of "string", "integer", or "float"
	hasSuffix    bool
	savedString  map[int]string
	savedInteger map[int]int
	savedFloat   map[int]float64
	// hasFactor bool
}

// LoadConfig loads and stores the configuration for this Integration
func (s *Scraper) LoadConfig(confdir string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not preprocess Scraper configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, s)
	if err != nil {
		log.Fatalf("ERROR: Could not load Scraper config due to %s\n", err.Error())
		s.mutex.Unlock()
		return err
	}
	for i, sc := range s.Scrape {
		numIx := len(sc.Indices)
		numSubs := len(sc.Subtopics)
		if numIx != numSubs {
			log.Printf("WARNING: Scraper - # Indices <> # Subtopics in %s\n", sc.Name)
			return errors.New("Scraper configuration error")
		}
		sc.savedFloat = make(map[int]float64, numIx)
		sc.savedInteger = make(map[int]int, numIx)
		sc.savedString = make(map[int]string, numIx)
		s.Scrape[i] = sc
	}
	s.scrapersByName = make(map[string]int)
	for i, sc := range s.Scrape {
		s.scrapersByName[sc.Name] = i
	}
	log.Printf("INFO: Scraper has %d scrapers configured\n", len(s.Scrape))
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (s *Scraper) ProvidesDeviceTypes() []string {
	return []string{"Scraper", "Query"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (s *Scraper) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	s.mq = mq
	for _, sc := range s.Scrape {
		go s.runScraper(sc)
	}
	// log.Printf("DEBUG: Scraper has started %d scraper(s)\n", len(s.scrapers))
	go s.monitorQueries()
}

func (s *Scraper) addStopChan() (ix int) {
	s.mutex.Lock()
	s.stopChans = append(s.stopChans, make(chan bool))
	ix = len(s.stopChans) - 1
	s.mutex.Unlock()
	return ix
}

// Stop terminates the Integration and all Goroutines it contains
func (s *Scraper) Stop() {
	for _, ch := range s.stopChans {
		ch <- true
	}
	log.Println("DEBUG: Scraper - All Goroutines should have stopped")
}

func (s *Scraper) runScraper(scr scraperT) {
	log.Printf("DEBUG: Scraper - starting %v\n", scr)
	c := colly.NewCollector()
	// for _, d := range scr.Details {
	c.OnHTML("body", func(e *colly.HTMLElement) {
		e.ForEach(scr.Selector, func(ix int, el *colly.HTMLElement) {
			a := el.Attr(scr.Attribute)
			// if _, wanted := scr.Indices[ix]; wanted {
			wanted := false
			for ind := range scr.Indices {
				if ind == ix {
					wanted = true
				}
			}
			if wanted {
				// log.Printf("DEBUG: Scraper found Selector %s, index %d, attribute %s\n", scr.Selector, ix, a)
				if len(scr.Suffix) > 0 {
					a = strings.TrimSuffix(a, scr.Suffix)
				}
				// if scr.hasFactor {

				// }
				s.mutex.Lock()
				switch scr.ValueType {
				case "float":
					floatVal, err := strconv.ParseFloat(a, 64)
					if err != nil {
						log.Printf("WARNING: Scraper could not convert value '%s' to float, ignoring\n", a)
					} else {
						scr.savedFloat[ix] = floatVal
					}
				case "integer":
					intVal, err := strconv.ParseInt(a, 10, 0)
					if err != nil {
						log.Printf("WARNING: Scraper could not convert value '%s' to integer, ignoring\n", a)
					} else {
						// log.Printf("DEBUG: Scraper ix: %d in scraper %s\n", ix, scr.Name)
						scr.savedInteger[ix] = int(intVal)
					}
				case "string":
					scr.savedString[ix] = a
				}
				t := mqttPrefix + scr.Name + "/" + scr.Subtopics[scr.Indices[ix]]
				s.mutex.Unlock()
				// log.Printf("DEBUG: ... would publish %s to topic %s\n", a, t)
				s.mq.PublishChan <- mqtt.MessageT{
					Topic:    t,
					Qos:      0,
					Retained: true,
					Payload:  a,
				}
			}
		})
	})
	// }
	sc := s.addStopChan()
	s.mutex.RLock()
	stopChan := s.stopChans[sc]
	interval := scr.Interval
	s.mutex.RUnlock()
	ticker := time.NewTicker(time.Duration(interval) * time.Second)

	for {
		c.Visit(scr.URL)
		// log.Println("DEBUG: Scraped finished Visit()")
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			continue
		}
	}
}

func (s *Scraper) monitorQueries() {
	sc := s.addStopChan()
	s.mutex.RLock()
	stopChan := s.stopChans[sc]
	s.mutex.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, subscriberName, events.QueryDeviceType, "+", "+")
	if err != nil {
		log.Fatalf("ERROR: Scraper Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: Scraper Query Monitor got %v\n", ev)
			switch ev.EventName {
			case events.FetchLast:
				var val interface{}
				s.mutex.RLock()
				switch s.Scrape[s.scrapersByName[ev.DeviceName]].ValueType {
				case "float":
					val = s.Scrape[s.scrapersByName[ev.DeviceName]].savedFloat
				case "integer":
					val = s.Scrape[s.scrapersByName[ev.DeviceName]].savedInteger
				case "string":
					val = s.Scrape[s.scrapersByName[ev.DeviceName]].savedString
				}
				s.mutex.RUnlock()
				ev.Value.(chan interface{}) <- val
			default:
				log.Printf("WARNING: Scraper received unknown query type %s\n", ev.EventName)
			}
		}
	}
}
