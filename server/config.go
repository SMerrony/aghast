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

package server

import (
	"errors"
	"log"
	"os"

	"github.com/pelletier/go-toml"
)

const mainConfigFilename = "/config.toml"

// A MainConfigT holds the top-level configuration details
type MainConfigT struct {
	SystemName          string
	LogEvents           bool
	Longitude, Latitude float64
	MqttBroker          string
	MqttPort            int
	MqttClientID        string
	Integrations        []string
	configDir           string
}

// CheckMainConfig performs a simple sanity check on the main config.toml and its directory
func CheckMainConfig(configDir string) error {
	mainConfig, err := toml.LoadFile(configDir + mainConfigFilename)
	if err != nil {
		log.Println("ERROR: Could not load main configuration ", err.Error())
		return err
	}
	systemName := mainConfig.Get("systemName")
	if systemName == nil {
		return errors.New("No 'systemName' specified in main configuration")
	}
	log.Printf("INFO: Checking main configuration for '%s'\n", systemName)

	if mainConfig.GetArray("integrations") == nil {
		return errors.New("No Integrations section in config, cannot run")
	}
	integrations := mainConfig.GetArray("integrations").([]string)
	if len(integrations) == 0 {
		return errors.New("No Integrations enabled, cannot run")
	}
	// there should be a config file for each Integration and the time Integration must be specified
	timeFound := false
	for _, i := range integrations {
		if _, err := os.Stat(configDir + "/" + i + ".toml"); err != nil {
			return errors.New("No config file found for Integration: " + i)
		}
		if i == "time" {
			timeFound = true
		}
	}
	if !timeFound {
		return errors.New("The 'time' Integration must be enabled")
	}
	return nil
}

// LoadMainConfig does what it says on the tin
func LoadMainConfig(configDir string) (MainConfigT, error) {
	var conf MainConfigT
	mainConfig, err := toml.LoadFile(configDir + mainConfigFilename)
	if err != nil {
		log.Println("ERROR: Could not load main configuration ", err.Error())
		return conf, err
	}
	conf.SystemName = mainConfig.Get("systemName").(string)
	conf.MqttBroker = mainConfig.Get("mqttBroker").(string)
	conf.MqttPort = int(mainConfig.Get("mqttPort").(int64))
	conf.MqttClientID = mainConfig.Get("mqttClientID").(string)
	conf.LogEvents = mainConfig.Get("logEvents").(bool)

	log.Printf("DEBUG: Main config for %s loaded, MQTT Broker is %s\n", conf.SystemName, conf.MqttBroker)

	if mainConfig.GetArray("integrations") == nil {
		return conf, errors.New("No Integrations section in config, cannot run")
	}
	conf.Integrations = mainConfig.GetArray("integrations").([]string)
	conf.configDir = configDir
	return conf, nil
}
