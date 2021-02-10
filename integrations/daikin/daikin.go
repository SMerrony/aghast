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

package daikin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const (
	configFilename     = "/daikin.toml"
	subscriberName     = "Daikin"
	udpPort            = ":30050"
	udpQuery           = "DAIKIN_UDP" + getBasicInfo
	mqttPrefix         = "aghast/daikin/"
	maxUnits           = 20
	scanTimeout        = 10 * time.Second
	inverterReqTimeout = 5 * time.Second
)

// The Daikin type encapsulates the 'Daikin' HVAC Integration.
type Daikin struct {
	invertersMu           sync.RWMutex
	inverters             map[string]inverterT // the ones we use
	invertersByLabel      map[string]string    // map into the above via label
	unconfiguredInverters []inverterT          // we found these, but they aren't configured
	evChan                chan events.EventT
	mqttChan              chan mqtt.MessageT
	stopChans             []chan bool // used for stopping Goroutines
	mq                    mqtt.MQTT
	httpReqClient         *http.Client
}

type inverterT struct {
	// MACAddress string
	Label   string
	found   bool // we have found the unit at some point
	online  bool // the unit is currently accesible
	address string
	basicInfo, remoteMethodInfo,
	modelInfo, controlInfo,
	sensorInfo infoMap
	// clientChan chan mqtt.MessageT
}

// confT fields exported for unmarshalling
type confT struct {
	Inverter []struct {
		MAC   string
		Label string
	}
}

// ControlInfoMsgT is the type of messaage sent out both as an Event and via MQTT
// containing the interesting Control state from the unit
type ControlInfoMsgT struct {
	Power      string
	Mode       int
	Stemp      float64
	Frate      string
	Fdir       int
	LastUpdate string // "HH:MM:SS"
}

type reqT string

// these are the known Daikin 'end-points'
const (
	getBasicInfo    = "/common/basic_info"
	getRemoteMethod = "/common/get_remote_method"
	getModelInfo    = "/aircon/get_model_info"
	getControlInfo  = "/aircon/get_control_info"
	getSchedTimer   = "/aircon/get_scdtimer"
	getSensorInfo   = "/aircon/get_sensor_info"
	getWeekPower    = "/aircon/get_week_power"
	getYearPower    = "/aircon/get_year_power"
	getWeekPowerExt = "/aircon/get_week_power_ex"
	getYearPowerExt = "/aircon/get_year_power_ex"
	setControlInfo  = "/aircon/set_control_info"
)

const setControlFmt = "?pow=%s&mode=%s&stemp=%s&f_rate=%s&f_dir=%s&shum=%s"

type infoT int

const (
	stringT infoT = iota
	urlEncStringT
	floatT
	intT
	yesNoBoolT
	zeroOneBoolT
)

type infoElement struct {
	name             string
	rawValue         []byte
	label            string
	infoType         infoT
	valueUnavailable bool
	stringValue      string
	boolValue        bool
	intValue         int64
	floatValue       float64
}

type infoMap map[string]infoElement

// LoadConfig loads and stores the configuration for this Integration
func (d *Daikin) LoadConfig(confdir string) error {
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Daikin configuration ", err.Error())
		return err
	}
	var tmpConf confT
	err = toml.Unmarshal(confBytes, &tmpConf)
	if err != nil {
		log.Fatalf("ERROR: Could not load Daikin config due to %s\n", err.Error())
		return err
	}
	d.invertersMu.Lock()
	defer d.invertersMu.Unlock()
	d.inverters = make(map[string]inverterT)
	d.invertersByLabel = make(map[string]string)
	for _, i := range tmpConf.Inverter {
		var inv inverterT
		inv.Label = i.Label
		d.inverters[i.MAC] = inv
		d.invertersByLabel[inv.Label] = i.MAC
	}
	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (d *Daikin) ProvidesDeviceTypes() []string {
	return []string{"Inverter", "Control", "Query"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (d *Daikin) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	d.evChan = evChan
	d.mqttChan = mq.PublishChan
	d.mq = mq
	d.httpReqClient = &http.Client{
		Timeout: inverterReqTimeout,
	}
	d.runDiscovery(maxUnits, scanTimeout)

	d.mqttChan <- mqtt.MessageT{
		Topic:    mqttPrefix + "status",
		Qos:      0,
		Retained: false,
		Payload:  "Daikin Starting",
	}

	go d.monitorUnits()
	go d.monitorClients()
	go d.monitorActions()
	go d.monitorQueries()
	go d.rerunDiscovery(maxUnits, scanTimeout)
}

// Stop terminates the Integration and all Goroutines it contains
func (d *Daikin) Stop() {
	for _, ch := range d.stopChans {
		ch <- true
	}
}

func (d *Daikin) runDiscovery(maxUnits int, scanTimeout time.Duration) {
	// scan the local network for Daikin Inverter units
	foundUnits := discoverDaikinUnits(maxUnits, scanTimeout)
	for _, foundUnit := range foundUnits {
		// is it in our configuration?
		macAddr := foundUnit.basicInfo["mac"].stringValue
		knownUnit, known := d.inverters[macAddr]
		if !known {
			log.Println("INFO: Discovered \033[33munconfigured\033[0m Daikin Inverter!")
			d.invertersMu.Lock()
			d.unconfiguredInverters = append(d.unconfiguredInverters, foundUnit)
			d.invertersMu.Unlock()
			log.Printf("INFO: ... IP - %s, MAC - %s, Name - %s\n", foundUnit.address, macAddr, foundUnit.basicInfo["name"].stringValue)
		} else {
			if !knownUnit.found {
				d.invertersMu.Lock()
				log.Println("INFO: Discovered configured Daikin Inverter")
				knownUnit.Label = d.inverters[macAddr].Label
				knownUnit.address = "http://" + strings.Split(foundUnit.address, ":")[0]
				knownUnit.found = true
				knownUnit.online = true
				d.inverters[macAddr] = knownUnit
				d.invertersMu.Unlock()
				log.Printf("INFO: ... IP - %s, MAC - %s, Name - %s\n", foundUnit.address, macAddr, foundUnit.basicInfo["name"].stringValue)
			}
		}
	}
}

func (d *Daikin) addStopChan() (ix int) {
	d.invertersMu.Lock()
	d.stopChans = append(d.stopChans, make(chan bool))
	ix = len(d.stopChans) - 1
	d.invertersMu.Unlock()
	return ix
}

func (d *Daikin) rerunDiscovery(maxUnits int, scanTimeout time.Duration) {
	sc := d.addStopChan()
	d.invertersMu.RLock()
	stopChan := d.stopChans[sc]
	d.invertersMu.RUnlock()
	every15mins := time.NewTicker(15 * time.Minute)
	for {
		select {
		case <-every15mins.C:
			d.runDiscovery(maxUnits, scanTimeout)
		case <-stopChan:
			return
		}
	}
}

// monitorClients waits for client (front-end user) events coming via MQTT and handles them
func (d *Daikin) monitorClients() {

	sc := d.addStopChan()
	d.invertersMu.RLock()
	stopChan := d.stopChans[sc]
	d.invertersMu.RUnlock()
	clientChan := d.mq.SubscribeToTopic(mqttPrefix + "client/#")
	for {
		select {
		case <-stopChan:
			return
		case msg := <-clientChan:
			payload := string(msg.Payload.([]uint8))
			log.Printf("DEBUG: Got msg from Daikin client, topic: %s, payload: %s\n", msg.Topic, payload)
			// topic format is aghast/daikin/client/<Label>/<control>
			topicSlice := strings.Split(msg.Topic, "/")
			unitMAC, found := d.invertersByLabel[topicSlice[3]]
			if !found {
				log.Printf("WARNING: Daikin front-end monitor got command for unknown unit <%s>\n", topicSlice[3])
				continue
			}

			// get the existing settings from the unit
			ci, err := d.requestControlInfo(unitMAC)
			if err != nil {
				log.Printf("WARNING: Could not retrieve Control info for unit: %s\n", topicSlice[3])
				continue
			}

			d.invertersMu.RLock()
			inv := d.inverters[unitMAC]
			d.invertersMu.RUnlock()

			control := topicSlice[4]
			var power, setting, mode, fan, sweep string

			if control == "power" {
				if payload == "true" {
					power = "1"
				} else {
					power = "0"
				}
			} else {
				if ci["pow"].boolValue {
					power = "1"
				} else {
					power = "0"
				}
			}

			if control == "setting" {
				setting = payload
			} else {
				setting = fmt.Sprintf("%.1f", ci["stemp"].floatValue)
			}

			if control == "mode" {
				mode = payload
			} else {
				mode = fmt.Sprintf("%d", ci["mode"].intValue)
			}

			if control == "fan" {
				fan = payload
			} else {
				fan = ci["f_rate"].stringValue
			}

			if control == "sweep" {
				sweep = payload
			} else {
				sweep = fmt.Sprintf("%d", ci["f_dir"].intValue)
			}

			cmd := fmt.Sprintf(setControlFmt, power, mode, setting, fan, sweep, "0")
			// send the command
			addr := inv.address
			//debugging
			log.Printf("DEBUG: Daikin Sending F/E-Client Control to: %s as: %s\n", inv.Label, cmd)
			resp, err := http.Get(addr + setControlInfo + cmd)

			if err != nil {
				log.Printf("WARNING: Daikin - error sending control command %v\v", err)
				continue
			}
			if resp.Status != "200 OK" {
				log.Printf("WARNING: Daikin control response from %s was %s\n", addr, resp.Status)
			}
			resp.Body.Close()

			// refresh our copy the unit data
			qci, err := d.requestControlInfo(unitMAC)
			if err == nil {
				inv.controlInfo = qci
			}
		}
	}
}

func (d *Daikin) monitorUnits() {
	sc := d.addStopChan()
	d.invertersMu.RLock()
	stopChan := d.stopChans[sc]
	d.invertersMu.RUnlock()
	everyMinute := time.NewTicker(time.Minute)
	for {
		for mac, unit := range d.inverters {
			if unit.found {
				si, err := d.requestSensorInfo(mac)
				if err != nil {
					log.Printf("INFO: Daikin Sensor probe of %s failed with %v\n", unit.Label, err)
					// log.Printf("... Unit data is %v\n", unit)
					unit.online = false

				} else {
					unit.online = true
					// log.Printf("DEBUG: ... Unit - %s, Unit Temp - %f, Outside Temp - %f\n", unit.Label, unit.sensorInfo["htemp"].floatValue, unit.sensorInfo["otemp"].floatValue)					d.evChan <- events.EventT{
					d.evChan <- events.EventT{
						Name:  "Daikin/Inverter/" + unit.Label + "/Temperature",
						Value: fmt.Sprintf("%.1f", si["htemp"].floatValue),
					}
					d.evChan <- events.EventT{
						Name:  "Daikin/Inverter/" + unit.Label + "/OutsideTemperature",
						Value: fmt.Sprintf("%.1f", si["otemp"].floatValue),
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    mqttPrefix + unit.Label + "/temperature",
						Qos:      0,
						Retained: true,
						Payload:  fmt.Sprintf("%.1f", si["htemp"].floatValue),
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    mqttPrefix + unit.Label + "/outsidetemperature",
						Qos:      0,
						Retained: true,
						Payload:  fmt.Sprintf("%.1f", si["otemp"].floatValue),
					}
					unit.sensorInfo = si
				}
				ci, err := d.requestControlInfo(mac)
				if err != nil {
					log.Printf("INFO: Daikin Control probe of %s failed with %v\n", unit.Label, err)
					// log.Printf("... Unit data is %v\n", unit)
					unit.online = false
				} else {
					unit.online = true
					var pubCi ControlInfoMsgT
					pubCi.Power = fmt.Sprintf("%v", ci["pow"].boolValue)
					pubCi.Mode = int(ci["mode"].intValue)
					pubCi.Stemp = ci["stemp"].floatValue
					pubCi.Frate = ci["f_rate"].stringValue
					pubCi.Fdir = int(ci["f_dir"].intValue)
					pubCi.LastUpdate = time.Now().Format("15:04:05")
					payload, err := json.Marshal(pubCi)
					if err != nil {
						panic(err)
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    mqttPrefix + unit.Label + "/controlinfo",
						Qos:      0,
						Retained: true,
						Payload:  payload,
					}

					unit.controlInfo = ci
				}
				// write the updated unit back into the map
				d.invertersMu.Lock()
				d.inverters[mac] = unit
				d.invertersMu.Unlock()
			}
		}
		select {
		case <-stopChan:
			return
		case <-everyMinute.C:
			continue
		}
	}
}

func (d *Daikin) monitorQueries() {
	sc := d.addStopChan()
	d.invertersMu.RLock()
	stopChan := d.stopChans[sc]
	d.invertersMu.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, subscriberName+"/"+events.QueryDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: Daikin Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: Daikin Query Monitor got %v\n", ev)
			switch strings.Split(ev.Name, "/")[events.EvQueryType] {
			case events.IsAvailable:
				var isAvailable bool
				d.invertersMu.RLock()
				dev := d.invertersByLabel[strings.Split(ev.Name, "/")[events.EvDeviceName]]
				isAvailable = d.inverters[dev].online
				log.Printf("DEBUG: Daikin - Queried %s and got %v\n", dev, isAvailable)
				d.invertersMu.RUnlock()
				ev.Value.(chan interface{}) <- isAvailable
			case events.IsOn:
				var isOn bool
				d.invertersMu.RLock()
				dev := d.invertersByLabel[strings.Split(ev.Name, "/")[events.EvDeviceName]]
				isOn = d.inverters[dev].controlInfo["pow"].boolValue
				log.Printf("DEBUG: Daikin - Queried %s and got %v\n", dev, isOn)
				d.invertersMu.RUnlock()
				ev.Value.(chan interface{}) <- isOn
			default:
				log.Printf("WARNING: Daikin received unknown query type %s\n", ev.Name)
			}
		}
	}
}

func getDeviceName(evName string) string {
	return strings.Split(evName, "/")[events.EvDeviceName]
}

// monitorActions listens for Control Actions from Automations and performs them
func (d *Daikin) monitorActions() {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("ERROR: Daikin Automation Action panicked.\n%v\nFix the Automation and restart AGHAST!", r)
			log.Print(msg)
			d.mqttChan <- mqtt.MessageT{
				Topic:    mqtt.StatusTopic,
				Qos:      0,
				Retained: true,
				Payload:  msg,
			}
		}
	}()
	sc := d.addStopChan()
	d.invertersMu.RLock()
	stopChan := d.stopChans[sc]
	d.invertersMu.RUnlock()
	sid := events.GetSubscriberID(subscriberName)
	ch, err := events.Subscribe(sid, "Daikin/"+events.ActionControlDeviceType+"/+/+")
	if err != nil {
		log.Fatalf("ERROR: Daikin Integration could not subscribe to event - %v\n", err)
	}
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			log.Printf("DEBUG: Daikin Action Monitor got %v\n", ev)
			unitMAC, found := d.invertersByLabel[getDeviceName(ev.Name)]
			if !found {
				log.Printf("WARNING: Daikin automation got Action for unknown unit <%s>\n", getDeviceName(ev.Name))
				continue
			}

			// get the existing settings from the unit
			ci, err := d.requestControlInfo(unitMAC)
			if err != nil {
				log.Printf("WARNING: Could not retrieve Control info for unit: %s\n", getDeviceName(ev.Name))
				continue
			}
			inv := d.inverters[unitMAC]
			control := strings.Split(ev.Name, "/")[events.EvControl]

			var power, setting, mode, fan, sweep string

			if control == "power" {
				if ev.Value.(string) == "on" {
					power = "1"
				} else {
					power = "0"
				}
			} else {
				if ci["pow"].boolValue {
					power = "1"
				} else {
					power = "0"
				}
			}

			if control == "temperature" {
				setting = fmt.Sprintf("%.1f", ev.Value.(float64))
			} else {
				setting = fmt.Sprintf("%.1f", ci["stemp"].floatValue)
			}

			if control == "mode" {
				modeInt, ok := stringToAcMode(ev.Value.(string))
				if !ok {
					log.Printf("WARNING: Daikin automation handler got invalid MODE: <%s>\n", ev.Value.(string))
					continue
				}
				mode = fmt.Sprintf("%d", modeInt)
			} else {
				mode = fmt.Sprintf("%d", ci["mode"].intValue)
			}

			if control == "fan" {
				fan = ev.Value.(string)
			} else {
				fan = ci["f_rate"].stringValue
			}

			sweep = fmt.Sprintf("%d", ci["f_dir"].intValue)

			cmd := fmt.Sprintf(setControlFmt, power, mode, setting, fan, sweep, "0")
			// send the command
			addr := d.inverters[unitMAC].address
			//debugging
			log.Printf("DEBUG: Daikin Sending Action Control to: %s as: %s\n", d.inverters[unitMAC].Label, cmd)
			resp, err := http.Get(addr + setControlInfo + cmd)

			if err != nil {
				log.Printf("WARNING: Daikin - error sending control command %v\v", err)
				continue
			}
			if resp.Status != "200 OK" {
				log.Printf("WARNING: Daikin control response from %s was %s\n", addr, resp.Status)
			}
			resp.Body.Close()

			// refresh our copy the unit data
			qci, err := d.requestControlInfo(unitMAC)
			if err == nil {
				inv.controlInfo = qci
			}
		}
	}
}

// discoverDaikinUnits searches for Inverters on the local network
func discoverDaikinUnits(maxUnits int, timeout time.Duration) (units []inverterT) {
	pc, err := net.ListenPacket("udp4", udpPort)
	if err != nil {
		panic(err)
	}
	defer pc.Close()
	addr, err := net.ResolveUDPAddr("udp4", "192.168.1.255"+udpPort)
	if err != nil {
		panic(err)
	}
	_, err = pc.WriteTo([]byte(udpQuery), addr)
	if err != nil {
		panic(err)
	}

	pc.SetReadDeadline(time.Now().Add(timeout))
	for u := 0; u < maxUnits; u++ {
		buf := make([]byte, 1024)
		n, addrX, err := pc.ReadFrom(buf)

		if err != nil {
			// normal timeout
			if strings.HasSuffix(err.Error(), "timeout") {
				return
			}
			panic(err)
		}
		if strings.HasSuffix(addrX.String(), udpPort) {
			// this is the request, not a response
			u--
		} else {
			//fmt.Printf("%s sent this: %s\n", addrX, buf[:n])
			var d inverterT
			d.address = addrX.String()
			d.prepareBasicInfo()
			if err = parseKnownInfo(buf[:n], d.basicInfo); err != nil {
				log.Printf("WARNING: Could not parse response: %s\n", buf[:n])
				u--
			} else {
				ret, found := d.basicInfo["ret"]
				if !found || ret.stringValue != "OK" {
					// not really a Daikin unit - something else responded to the broadcast
					u--
				} else {
					units = append(units, d)
				}
			}
		}
	}
	return units
}

// func (du *inverterT) requestSensorInfo() (e error) {
func (d *Daikin) requestSensorInfo(mac string) (si infoMap, e error) {
	si = infoMap{
		"ret":     {label: "Return Code", infoType: stringT},
		"htemp":   {label: "Current Temperature", infoType: floatT},
		"hhum":    {label: "Current Humidity", infoType: floatT},
		"otemp":   {label: "Outside Temperature", infoType: floatT},
		"err":     {label: "Error Code", infoType: intT},
		"cmpfreq": {label: "Compressor Freq.", infoType: intT},
	}
	e = d.requestInfo(mac, getSensorInfo, si)
	return si, e
}

func (d *Daikin) requestControlInfo(mac string) (ci infoMap, e error) {
	ci = infoMap{
		"ret":    {label: "Return Code", infoType: stringT},
		"pow":    {label: "Power", infoType: zeroOneBoolT},
		"mode":   {label: "Mode", infoType: intT},
		"stemp":  {label: "Set Temperature", infoType: floatT},
		"shum":   {label: "Set Humidity", infoType: floatT},
		"alert":  {label: "Alert", infoType: intT},
		"f_rate": {label: "Fan Rate", infoType: stringT},
		"f_dir":  {label: "Fan Sweep", infoType: intT},
	}
	e = d.requestInfo(mac, getControlInfo, ci)
	return ci, e
}

// requestInfo sends an HTTP Get request to the unit provided and attempts
// to parse the response.  It is internal to this package.
func (d *Daikin) requestInfo(mac string, req string, dest infoMap) (e error) {
	d.invertersMu.RLock()
	du := d.inverters[mac]
	resp, err := d.httpReqClient.Get(du.address + req)
	d.invertersMu.RUnlock()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// fmt.Printf("DEBUG: Raw response: %s\n", string(body))
	if e = parseKnownInfo(body, dest); e != nil {
		return e
	}
	return nil
}

// func (d *Daikin) setControls(mac string, ci ControlInfoMsgT) error {
// 	v := url.Values{}
// 	v.Set("pow", ci.Power)
// 	v.Add("mode", fmt.Sprintf("%d", ci.Mode))
// 	v.Add("stemp", fmt.Sprintf("%.1f", ci.Stemp))
// 	v.Add("f_rate", ci.Frate)
// 	v.Add("f_dir", fmt.Sprintf("%d", ci.Fdir))

// 	resp, err := http.PostForm(d.inverters[mac].address+setControlInfo, v)
// 	if err != nil {
// 		return err
// 	}
// 	resp.Body.Close()
// 	return nil
// }

func (du *inverterT) prepareBasicInfo() {
	if len(du.basicInfo) == 0 {
		du.basicInfo = infoMap{
			"ret":        {label: "Return Code", infoType: stringT},
			"type":       {label: "Unit Type", infoType: stringT},
			"reg":        {label: "Region", infoType: stringT},
			"dst":        {label: "Automatic DST", infoType: zeroOneBoolT},
			"ver":        {label: "Adaptor Firmware Version", infoType: stringT},
			"rev":        {label: "Adaptor Revision", infoType: stringT},
			"err":        {label: "Error Code", infoType: intT},
			"location":   {label: "Location", infoType: intT},
			"name":       {label: "Name", infoType: urlEncStringT},
			"icon":       {label: "Icon", infoType: intT},
			"method":     {label: "Method", infoType: stringT},
			"port":       {label: "Port", infoType: intT},
			"id":         {label: "User ID", infoType: stringT},
			"pw":         {label: "Password", infoType: stringT},
			"lpw_flag":   {label: "L. Password", infoType: zeroOneBoolT},
			"pow":        {label: "Power On", infoType: zeroOneBoolT},
			"adp_kind":   {label: "Adaptor Type", infoType: intT},
			"pv":         {label: "Adaptor H/W Version", infoType: floatT},
			"cpv":        {label: "Adaptor H/W Major Version", infoType: intT},
			"cpv_minor":  {label: "Adaptor H/W Minor Version", infoType: intT},
			"led":        {label: "Adaptor LEDs", infoType: zeroOneBoolT},
			"en_setzone": {label: "Adaptor Zone Set", infoType: zeroOneBoolT},
			"mac":        {label: "Adaptor MAC Address", infoType: stringT},
			"adp_mode":   {label: "Adaptor Mode", infoType: stringT},
			"en_hol":     {label: "Holiday Mode", infoType: zeroOneBoolT},
			"en_grp":     {label: "Group Mode", infoType: zeroOneBoolT},
			"grp_name":   {label: "Group", infoType: stringT},
		}
	}
}

// parseKnownInfo extracts known key/value pairs and populates the given map
func parseKnownInfo(body []byte, im infoMap) (e error) {
	var err error
	splitBA := bytes.Split(body, []byte(","))
	for _, v := range splitBA {
		elements := bytes.Split(v, []byte("="))
		info, known := im[string(elements[0])]
		if known {

			switch info.infoType {
			case stringT:
				if len(elements[1]) == 0 {
					info.stringValue = ""
					info.valueUnavailable = true
				} else {
					info.stringValue = string(elements[1])
				}
				// bail out if ret is not OK
				if info.label == "Return Code" && info.stringValue != "OK" {
					return errors.New("Invalid Request")
				}

			case urlEncStringT:
				info.stringValue, err = url.QueryUnescape(string(elements[1]))
				if err != nil {
					log.Printf("WARNING: Could not unescape string from %s\n", string(elements[1]))
				}

			case intT:
				info.intValue, err = strconv.ParseInt(string(elements[1]), 10, 64)
				if err != nil {
					log.Printf("WARNING: Could not parse int from %s\n", string(elements[1]))
				}

			case floatT:
				if len(elements[1]) < 3 && elements[1][0] == '-' {
					info.floatValue = 0.0
					info.valueUnavailable = true
				} else {
					info.floatValue, err = strconv.ParseFloat(string(elements[1]), 64)
					if err != nil {
						log.Printf("WARNING: Could not parse float from %s\n", string(elements[1]))
					}
				}

			case zeroOneBoolT:
				if elements[1][0] == '1' {
					info.boolValue = true
				} else {
					info.boolValue = false
				}

			}
			im[string(elements[0])] = info
		} else {
			// fmt.Printf("%s\n", v)
		}
	}
	return nil
}

var acModes = []string{"Auto", "Auto 1", "Dehumidify", "Cool", "Heat", "Mode 5", "Fan", "Auto 2"}

func acModeToString(mode int) (string, bool) {
	return internalIntToString(acModes, mode)
}

func stringToAcMode(mode string) (int, bool) {
	for i, v := range acModes {
		if v == mode {
			return i, true
		}
	}
	return 0, false
}

var fanRates = map[string]string{
	"A": "Auto",
	"B": "Silent",
	"3": "Level 1",
	"4": "Level 2",
	"5": "Level 3",
	"6": "Level 4",
	"7": "Level 5",
}

func fanRateToString(rate string) (string, bool) {
	s, ok := fanRates[rate]
	return s, ok
}

var fanSweeps = []string{"Off", "Vertical", "Horizontal", "3D"}

func fanSweepToString(swp int) (string, bool) {
	return internalIntToString(fanSweeps, swp)
}

// helper funcs...

func internalIntToString(arr []string, ix int) (string, bool) {
	if ix < len(arr) {
		return arr[ix], true
	}
	return "", false
}
