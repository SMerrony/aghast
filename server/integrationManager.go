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
	"html/template"
	"log"
	"net/http"
	"runtime"
	"strconv"
	gotime "time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/integrations/automation"
	"github.com/SMerrony/aghast/integrations/datalogger"
	"github.com/SMerrony/aghast/integrations/hostchecker"
	"github.com/SMerrony/aghast/integrations/influx"
	"github.com/SMerrony/aghast/integrations/mqttcache"
	"github.com/SMerrony/aghast/integrations/mqttsender"
	"github.com/SMerrony/aghast/integrations/pimqttgpio"
	"github.com/SMerrony/aghast/integrations/postgres"
	"github.com/SMerrony/aghast/integrations/scraper"
	"github.com/SMerrony/aghast/integrations/time"
	"github.com/SMerrony/aghast/integrations/tuya"
	"github.com/SMerrony/aghast/integrations/zigbee2mqtt"
	"github.com/SMerrony/aghast/mqtt"
)

// The Integration interface defines the minimal set of methods that an
// Integration must provide
type Integration interface {
	// LoadConfig func should simply load any config (TOML) files for this Integration
	LoadConfig(string) error

	// Start func begins running the Integration GoRoutines and should return quickly
	Start(chan events.EventT, mqtt.MQTT)

	// Stop terminates the Integration and all Goroutines it contains
	Stop()
}

var integs = make(map[string]Integration)
var mainConfig config.MainConfigT
var evCh chan events.EventT
var mq mqtt.MQTT

func newIntegration(iName string) {
	switch iName {
	case "automation":
		integs[iName] = new(automation.Automation)
	case "datalogger":
		integs[iName] = new(datalogger.DataLogger)
	case "hostchecker":
		integs[iName] = new(hostchecker.HostChecker)
	case "influx":
		integs[iName] = new(influx.Influx)
	case "mqttcache":
		integs[iName] = new(mqttcache.MqttCache)
	case "mqttsender":
		integs[iName] = new(mqttsender.MqttSender)
	case "pimqttgpio":
		integs[iName] = new(pimqttgpio.PiMqttGpio)
	case "postgres":
		integs[iName] = new(postgres.Postgres)
	case "scraper":
		integs[iName] = new(scraper.Scraper)
	case "time":
		integs[iName] = new(time.Time)
	case "tuya":
		integs[iName] = new(tuya.Tuya)
	case "zigbee2mqtt":
		integs[iName] = new(zigbee2mqtt.Zigbee2MQTT)
	default:
		log.Fatalf("ERROR: Integration '%s' is not known\n", iName)
	}
}

// StartIntegrations asks each enabled Integration to configure itself, then starts them.
func StartIntegrations(conf config.MainConfigT, evChan chan events.EventT, mqtt mqtt.MQTT) {
	mainConfig = conf
	evCh = evChan
	mq = mqtt
	for _, i := range conf.Integrations {
		newIntegration(i)
		if err := integs[i].LoadConfig(conf.ConfigDir); err != nil {
			log.Fatalf("ERROR: %s Integration could not load its configuration", i)
		}
		go integs[i].Start(evChan, mqtt)
	}

	go dailyTimeRestart()

	// start a HTTP server for back-end control
	http.HandleFunc("/", rootHandler)
	if err := http.ListenAndServe(":"+strconv.Itoa(conf.ControlPort), nil); err != nil {
		log.Println("WARNING: Could not start HTTP admin control back-end")
	}
}

const homeTemplateMain = `<!DOCTYPE html>
<html>
 <head>
  <link rel="icon" type="image/png" href="data:image/png;base64,iVBORw0KGgo=">
  <title>AGHAST - {{.SystemName}}</title>
  <style>
  body {
	background-color: AliceBlue;
	font-family: Arial, Helvetica, sans-serif;
  }
  th, td {
	padding: 5px;
  }
  </style>
 </head>
 <body>
  <h1>AGHAST - {{.SystemName}}</h1>
  <p>Configuration directory: <samp>{{.ConfigDir}}</samp></p>
  <p>MQTT Broker: <samp>{{.MqttBroker}}</samp></p>
  <h2>Configured Integrations</h2>
   <p>You can reload an Integration's configuration here (it will be stopped, reloaded, and restarted).</p>
   <p>You can also completely stop an Integration that is causing problems; there is no way to restart it other than
	  stopping the AGHAST server and restarting it.  Take care!</p>
   <form method="POST">
	<table>
		<tr><th>Integration</th><th></th><th></th></tr>
		{{range .Integrations}}
		<tr>
		 <td>{{.}}</td>
		 <td><button name="reload" value="{{.}}">Reload</button></td>
		 <td><button name="stop" value="{{.}}">Stop</button></td>
		</tr>
		{{end}}
	</table>
   </form>
`

const homeTemplateStats = `
  <h2>Statistics</h2>
   <table style="text-align: center">
	<tr><th>Memory Allocated (MB)</th><th>No. Goroutines</th></tr>
	<tr><td>{{.TotalMemoryMB}}</td><td>{{.NumGoroutines}}</td></tr>
   </table>
   <input type="button" value="Update Data" onClick="location.href=location.href">
 </body>
</html>`

type sysStatsT struct {
	TotalMemoryMB uint64
	NumGoroutines int
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	// log.Printf("DEBUG: HTTP rootHandler got stop for: %s\n", r.FormValue("stop"))
	if r.FormValue("stop") != "" {
		i := r.FormValue("stop")
		integs[i].Stop()
		delete(integs, i)
		for ix, in := range mainConfig.Integrations {
			if in == i {
				copy(mainConfig.Integrations[ix:], mainConfig.Integrations[ix+1:])
				mainConfig.Integrations[len(mainConfig.Integrations)-1] = ""
				mainConfig.Integrations = mainConfig.Integrations[:len(mainConfig.Integrations)-1]
			}
		}
	}
	// log.Printf("DEBUG: HTTP rootHandler got reload for : %s\n", r.FormValue("reload"))
	if r.FormValue("reload") != "" {
		i := r.FormValue("reload")
		integs[i].Stop()
		newIntegration(i)
		if err := integs[i].LoadConfig(mainConfig.ConfigDir); err != nil {
			log.Fatalf("ERROR: %s Integration could not reload its configuration", i)
		}
		go integs[i].Start(evCh, mq)
	}
	t, err := template.New("root").Parse(homeTemplateMain)
	if err != nil {
		log.Fatalf("ERROR: Could not parse root admin template - this should not happen!")
	}
	err = t.Execute(w, mainConfig)

	var sysStats sysStatsT
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	sysStats.TotalMemoryMB = memStats.Sys >> 20
	sysStats.NumGoroutines = runtime.NumGoroutine()
	t2, err := template.New("root2").Parse(homeTemplateStats)
	err = t2.Execute(w, sysStats)
	log.Println("DEBUG: HTTP Back-end generated a page")
}

func dailyTimeRestart() {
	// wait until 1st restart time (01:05hrs)
	now := gotime.Now()
	yyyy, mm, dd := now.Date()
	reloadTime := gotime.Date(yyyy, mm, dd+1, 1, 5, 0, 0, now.Location())
	untilRealoadTime := reloadTime.Sub(now)
	timer := gotime.NewTimer(untilRealoadTime)
	<-timer.C
	// restart every 24 hours
	daily := gotime.NewTicker(gotime.Hour * 24)
	for {
		log.Println("INFO: Daily Time Integration reload")
		integs["time"].Stop()
		newIntegration("time")
		if err := integs["time"].LoadConfig(mainConfig.ConfigDir); err != nil {
			log.Fatalln("ERROR: Time Integration could not reload its configuration")
		}
		go integs["time"].Start(evCh, mq)
		<-daily.C
	}

}
