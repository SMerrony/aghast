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

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/pelletier/go-toml"
)

const configFilename = "/daikin.toml"

// The Daikin type encapsulates the 'Daikin' HVAC Integration.
type Daikin struct {
	invertersMu           sync.RWMutex
	inverters             map[string]inverterT // the ones we use
	unconfiguredInverters []inverterT          // we found these, but they aren't configured
	evChan                chan events.EventT
	mqttChan              chan mqtt.MessageT
}

type MQTTControlInfoT struct {
	Power string
	Mode  int
	Stemp float64
	Frate string
	Fdir  int
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
)

const (
	udpPort     = ":30050"
	udpQuery    = "DAIKIN_UDP" + getBasicInfo
	maxUnits    = 20
	scanTimeout = 10 * time.Second
)

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

type inverterT struct {
	// MACAddress string
	Label   string
	online  bool
	address string
	basicInfo, remoteMethodInfo,
	modelInfo, controlInfo,
	sensorInfo infoMap
}

// LoadConfig loads and stores the configuration for this Integration
func (d *Daikin) LoadConfig(confdir string) error {
	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load configuration ", err.Error())
		return err
	}
	d.invertersMu.Lock()
	defer d.invertersMu.Unlock()
	d.inverters = make(map[string]inverterT)

	confMap := conf.ToMap()
	invsConf := confMap["Inverter"].(map[string]interface{})
	for mac, i := range invsConf {
		var inv inverterT
		details := i.(map[string]interface{})
		// inv.MACAddress = mac
		inv.Label = details["label"].(string)
		d.inverters[mac] = inv
		log.Printf("DEBUG: Configured Daikin Inverter at %s as %s\n", mac, inv.Label)
	}

	return nil
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (d *Daikin) ProvidesDeviceTypes() []string {
	return []string{"Inverter"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (d *Daikin) Start(evChan chan events.EventT, mqChan chan mqtt.MessageT) {

	d.evChan = evChan
	d.mqttChan = mqChan

	d.runDiscovery(maxUnits, scanTimeout)

	msg := mqtt.MessageT{
		Topic:    "daikin/status",
		Qos:      0,
		Retained: false,
		Payload:  "Daikin Starting",
	}
	d.mqttChan <- msg
	// configured and accessible units are now in d, we can start monitoring etc.
	go d.monitor()
	go d.rerunDiscovery(maxUnits, scanTimeout)
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
		} else {
			d.invertersMu.Lock()
			// if _, loaded := d.inverters[macAddr]; !loaded {
			log.Println("INFO: Discovered configured Daikin Inverter")
			knownUnit.Label = d.inverters[macAddr].Label
			knownUnit.address = "http://" + strings.Split(foundUnit.address, ":")[0]
			knownUnit.online = true
			d.inverters[macAddr] = knownUnit
			// }
			d.invertersMu.Unlock()
		}
		log.Printf("INFO: ... IP - %s, MAC - %s, Name - %s\n", foundUnit.address, macAddr, foundUnit.basicInfo["name"].stringValue)
	}
}

func (d *Daikin) rerunDiscovery(maxUnits int, scanTimeout time.Duration) {
	every15mins := time.NewTicker(15 * time.Minute)
	for {
		<-every15mins.C
		d.runDiscovery(maxUnits, scanTimeout)
	}
}

func (d *Daikin) monitor() {
	everyMinute := time.NewTicker(time.Minute)
	for {
		// <-everyMinute.C
		// log.Println("DEBUG: Running Daikin monitor probe")
		for _, unit := range d.inverters {
			if unit.online {
				err := unit.requestSensorInfo()
				if err != nil {
					log.Printf("WARNING: Daikin sensor probe failed with %v\n", err)
					log.Printf("... Unit data is %v\n", unit)
					unit.online = false
				} else {
					// log.Printf("DEBUG: ... Unit - %s, Unit Temp - %f, Outside Temp - %f\n", unit.Label, unit.sensorInfo["htemp"].floatValue, unit.sensorInfo["otemp"].floatValue)
					d.evChan <- events.EventT{
						Integration: "Daikin",
						DeviceType:  "Inverter",
						DeviceName:  unit.Label,
						EventName:   "Temperature",
						Value:       fmt.Sprintf("%.1f", unit.sensorInfo["htemp"].floatValue),
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    "daikin/" + unit.Label + "/temperature",
						Qos:      0,
						Retained: true,
						Payload:  fmt.Sprintf("%.1f", unit.sensorInfo["htemp"].floatValue),
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    "daikin/" + unit.Label + "/outsidetemperature",
						Qos:      0,
						Retained: true,
						Payload:  fmt.Sprintf("%.1f", unit.sensorInfo["otemp"].floatValue),
					}
				}
				err = unit.requestControlInfo()
				if err != nil {
					log.Printf("WARNING: Daikin control probe failed with %v\n", err)
					log.Printf("... Unit data is %v\n", unit)
					unit.online = false
				} else {
					d.evChan <- events.EventT{
						Integration: "Daikin",
						DeviceType:  "Inverter",
						DeviceName:  unit.Label,
						EventName:   "Power",
						Value:       fmt.Sprintf("%v", unit.controlInfo["pow"].boolValue),
					}

					var payloadStruct MQTTControlInfoT
					payloadStruct.Power = fmt.Sprintf("%v", unit.controlInfo["pow"].boolValue)
					payloadStruct.Mode = int(unit.controlInfo["mode"].intValue)
					payloadStruct.Stemp = unit.controlInfo["stemp"].floatValue
					payloadStruct.Frate = unit.controlInfo["f_rate"].stringValue
					payloadStruct.Fdir = int(unit.controlInfo["f_dir"].intValue)
					payload, err := json.Marshal(payloadStruct)
					if err != nil {
						panic(err)
					}
					d.mqttChan <- mqtt.MessageT{
						Topic:    "daikin/" + unit.Label + "/controlinfo",
						Qos:      0,
						Retained: true,
						Payload:  payload,
					}

					d.evChan <- events.EventT{
						Integration: "Daikin",
						DeviceType:  "Inverter",
						DeviceName:  unit.Label,
						EventName:   "Mode",
						Value:       fmt.Sprintf("%d", unit.controlInfo["mode"].intValue),
					}

					d.evChan <- events.EventT{
						Integration: "Daikin",
						DeviceType:  "Inverter",
						DeviceName:  unit.Label,
						EventName:   "SetTemperature",
						Value:       fmt.Sprintf("%.1f", unit.controlInfo["stemp"].floatValue),
					}

					d.evChan <- events.EventT{
						Integration: "Daikin",
						DeviceType:  "Inverter",
						DeviceName:  unit.Label,
						EventName:   "FanRate",
						Value:       fmt.Sprintf("%s", unit.controlInfo["f_rate"].stringValue),
					}

				}
			}
		}
		<-everyMinute.C
	}
}

// func (d *Daikin) publishFloat(evName, subTopic string, key string, floatVal float64) {
// 	d.evChan <- events.EventT{
// 		Integration: "Daikin",
// 		DeviceType:  "Inverter",
// 		DeviceName:  unit.Label,
// 		EventName:   evName,
// 		Value:       fmt.Sprintf("%.1f", floatVal),
// 	}
// 	d.mqttChan <- mqtt.MessageT{
// 		Topic:    "daikin/" + unit.Label + subTopic,
// 		Qos:      0,
// 		Retained: true,
// 		Payload:  fmt.Sprintf("%.1f", floatVal),
// 	}
// }

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
			// fmt.Printf("%s sent this: %s\n", addrX, buf[:n])
			var d inverterT
			d.address = addrX.String()
			d.prepareBasicInfo()
			if err = parseKnownInfo(buf[:n], d.basicInfo); err != nil {
				log.Printf("WARNING: Could not parse response: %s\n", buf[:n])
			} else {
				units = append(units, d)
			}
		}
	}
	return units
}

func (du *inverterT) requestSensorInfo() (e error) {
	if len(du.sensorInfo) == 0 {
		du.sensorInfo = infoMap{
			"ret":     {label: "Return Code", infoType: stringT},
			"htemp":   {label: "Current Temperature", infoType: floatT},
			"hhum":    {label: "Current Humidity", infoType: floatT},
			"otemp":   {label: "Outside Temperature", infoType: floatT},
			"err":     {label: "Error Code", infoType: intT},
			"cmpfreq": {label: "Compressor Freq.", infoType: intT},
		}
	}
	e = du.requestInfo(getSensorInfo, du.sensorInfo)
	return e
}

func (du *inverterT) requestControlInfo() (e error) {
	if len(du.controlInfo) == 0 {
		du.controlInfo = infoMap{
			"ret":    {label: "Return Code", infoType: stringT},
			"pow":    {label: "Power", infoType: zeroOneBoolT},
			"mode":   {label: "Mode", infoType: intT},
			"stemp":  {label: "Set Temperature", infoType: floatT},
			"shum":   {label: "Set Humidity", infoType: floatT},
			"alert":  {label: "Alert", infoType: intT},
			"f_rate": {label: "Fan Rate", infoType: stringT},
			"f_dir":  {label: "Fan Sweep", infoType: intT},
		}
	}
	e = du.requestInfo(getControlInfo, du.controlInfo)
	return e
}

// requestInfo sends an HTTP Get request to the unit provided and attempts
// to parse the response.  It is internal to this package.
func (du *inverterT) requestInfo(req string, dest infoMap) (e error) {
	resp, err := http.Get(du.address + req)
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
			// fmt.Printf("%s\t", info.label)
			switch info.infoType {
			case stringT:
				if len(elements[1]) == 0 {
					info.stringValue = ""
					info.valueUnavailable = true
					// fmt.Printf(" \tUnavailable ")
				} else {
					info.stringValue = string(elements[1])
				}
				// bail out if ret is not OK
				if info.label == "Return Code" && info.stringValue != "OK" {
					return errors.New("Invalid Request")
				}
				// if info.label == "Fan Rate" {
				// 	if s, ok := fanRateToString(info.stringValue); ok {
				// 		fmt.Printf(" \tDecodes to: %s ", s)
				// 	}
				// }
				// fmt.Printf(" \t\"%s\"\n", info.stringValue)
			case urlEncStringT:
				info.stringValue, err = url.QueryUnescape(string(elements[1]))
				if err != nil {
					log.Printf("WARNING: Could not unescape string from %s\n", string(elements[1]))
				}
				// fmt.Printf(" \t\"%s\"\n", info.stringValue)
			case intT:
				info.intValue, err = strconv.ParseInt(string(elements[1]), 10, 64)
				if err != nil {
					log.Printf("WARNING: Could not parse int from %s\n", string(elements[1]))
				}
				// if info.label == "Mode" {
				// 	if s, ok := acModeToString(int(info.intValue)); ok {
				// 		fmt.Printf(" \tDecodes to: %s ", s)
				// 	}
				// }
				// if info.label == "Fan Sweep" {
				// 	if s, ok := fanSweepToString(int(info.intValue)); ok {
				// 		fmt.Printf(" \tDecodes to: %s ", s)
				// 	}
				// }
				// fmt.Printf(" \t%d\n", info.intValue)
			case floatT:
				if len(elements[1]) < 3 && elements[1][0] == '-' {
					info.floatValue = 0.0
					info.valueUnavailable = true
					// fmt.Printf(" \tUnavailable ")
				} else {
					info.floatValue, err = strconv.ParseFloat(string(elements[1]), 64)
					if err != nil {
						log.Printf("WARNING: Could not parse float from %s\n", string(elements[1]))
					}
				}
				// fmt.Printf(" \t%f\n", info.floatValue)
			case zeroOneBoolT:
				if elements[1][0] == '1' {
					info.boolValue = true
				} else {
					info.boolValue = false
				}
				// fmt.Printf("\t%v\n", info.boolValue)
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
