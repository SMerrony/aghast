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
	"log"
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
)

// The Scraper type encapsulates the web scraper Integration.
type Scraper struct {
	mq        mqtt.MQTT
	scraperMu sync.RWMutex
	Scrape    []scraperT
	stopChans []chan bool // used for stopping Goroutines
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
	Suffix    string
	hasSuffix bool
	// hasFactor bool
}

// LoadConfig loads and stores the configuration for this Integration
func (s *Scraper) LoadConfig(confdir string) error {
	s.scraperMu.Lock()
	defer s.scraperMu.Unlock()

	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not preprocess Scraper configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, s)
	if err != nil {
		log.Fatalf("ERROR: Could not load Scraper config due to %s\n", err.Error())
		s.scraperMu.Unlock()
		return err
	}
	log.Printf("INFO: Scraper has %d scrapers configured\n", len(s.Scrape))
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (s *Scraper) ProvidesDeviceTypes() []string {
	return []string{"Scraper"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (s *Scraper) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	s.mq = mq
	for _, sc := range s.Scrape {
		go s.runScraper(sc)
	}
	// log.Printf("DEBUG: Scraper has started %d scraper(s)\n", len(s.scrapers))
}

func (s *Scraper) addStopChan() (ix int) {
	s.scraperMu.Lock()
	s.stopChans = append(s.stopChans, make(chan bool))
	ix = len(s.stopChans) - 1
	s.scraperMu.Unlock()
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
				t := mqttPrefix + scr.Name + "/" + scr.Subtopics[scr.Indices[ix]]
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
	s.scraperMu.RLock()
	stopChan := s.stopChans[sc]
	interval := scr.Interval
	s.scraperMu.RUnlock()
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
