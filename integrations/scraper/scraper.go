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

package scraper

import (
	"log"
	"strings"
	"time"

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
	mq       mqtt.MQTT
	scrapers map[string]scraperT
}

type scraperT struct {
	URL      string
	Interval int
	Details  []detailsT
}

type detailsT struct {
	Selector  string
	Attribute string
	Index     int
	Indices   map[int]int
	Subtopic  string
	Subtopics []string
	// Factor    float64
	Suffix    string
	hasSuffix bool
	// hasFactor bool
}

// LoadConfig loads and stores the configuration for this Integration
func (s *Scraper) LoadConfig(confdir string) error {

	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Scraper configuration ", err.Error())
		return err
	}
	s.scrapers = make(map[string]scraperT)
	confMap := conf.ToMap()
	scrapers := conf.Keys()

	for _, name := range scrapers {
		log.Printf("DEBUG: Scraper Loading config for %s\n", name)
		var scr scraperT
		sconf := confMap[name].(map[string]interface{})
		scr.URL = sconf["url"].(string)
		scr.Interval = int(sconf["interval"].(int64))
		dets := sconf["details"].([]interface{})
		for _, d := range dets {
			var det detailsT
			ixNum := 0
			dmap := d.(map[string]interface{})
			det.Selector = dmap["selector"].(string)
			det.Attribute = dmap["attribute"].(string)
			if _, ix := dmap["index"]; ix {
				det.Index = int(dmap["index"].(int64))
			}
			if _, ixs := dmap["indices"]; ixs {
				tmpIxs := dmap["indices"].([]interface{})
				det.Indices = make(map[int]int)
				for _, i := range tmpIxs {
					det.Indices[int(i.(int64))] = ixNum
					ixNum++
				}
			}
			// _, det.hasFactor = dmap["factor"]
			// if det.hasFactor {
			// 	det.Factor = dmap["factor"].(float64)
			// }
			if _, st := dmap["subtopic"]; st {
				det.Subtopic = dmap["subtopic"].(string)
			}
			if _, sts := dmap["subtopics"]; sts {
				tmpSts := dmap["subtopics"].([]interface{})
				for _, st := range tmpSts {
					det.Subtopics = append(det.Subtopics, st.(string))
				}
			}
			_, det.hasSuffix = dmap["suffix"]
			if det.hasSuffix {
				det.Suffix = dmap["suffix"].(string)
			}

			scr.Details = append(scr.Details, det)
		}
		s.scrapers[name] = scr
	}

	// c, err := ioutil.ReadFile(confdir + configFilename)
	// if err != nil {
	// 	log.Println("ERROR: Could not read Scraper configuration ", err.Error())
	// 	return err
	// }
	// err = toml.Unmarshal(c, &s.scrapers)
	// if err != nil {
	// 	log.Printf("ERROR: Could not parse Scraper config due to %v\n", err)
	// }

	// log.Printf("DEBUG: Scraper config... %v\n", s.scrapers)
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (s *Scraper) ProvidesDeviceTypes() []string {
	return []string{"Scraper"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (s *Scraper) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	s.mq = mq
	for sc := range s.scrapers {
		go s.scraper(sc)
	}
	log.Printf("DEBUG: Scraper has started %d scraper(s)\n", len(s.scrapers))
}

func (s *Scraper) scraper(name string) {
	scr := s.scrapers[name]
	c := colly.NewCollector()
	for _, d := range scr.Details {
		c.OnHTML("body", func(e *colly.HTMLElement) {
			e.ForEach(d.Selector, func(ix int, el *colly.HTMLElement) {
				a := el.Attr(d.Attribute)
				if _, wanted := d.Indices[ix]; wanted {
					// log.Printf("DEBUG: Scraper found Selector %s, index %d, attribute %s\n",
					// d.Selector, ix, a)
					if d.hasSuffix {
						a = strings.TrimSuffix(a, d.Suffix)
					}
					// if d.hasFactor {

					// }
					t := mqttPrefix + name + "/" + d.Subtopics[d.Indices[ix]]
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
	}
	for {
		c.Visit(scr.URL)
		// log.Printf("DEBUG: Scraper will sleep for %d seconds\n", scr.Interval)
		time.Sleep(time.Second * time.Duration(scr.Interval))
	}
}
