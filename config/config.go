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

package config

import (
	"errors"
	"log"
	"os"
	"reflect"

	"github.com/pelletier/go-toml"
)

const (
	mainConfigFilename = "/config.toml"
	secretsFilename    = "/secrets.toml"
	constantsFilename  = "/constants.toml"
	secretLabel        = "!!SECRET!!"
	constantLabel      = "!!CONSTANT!!"
)

// A MainConfigT holds the top-level configuration details
type MainConfigT struct {
	SystemName          string
	LogEvents           bool
	Longitude, Latitude float64
	MqttBroker          string
	MqttPort            int
	MqttClientID        string
	Integrations        []string
	ConfigDir           string
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
	conf.SystemName = GetString(configDir, mainConfig, "systemName")
	conf.Longitude = GetFloat64(configDir, mainConfig, "longitude")
	conf.Latitude = GetFloat64(configDir, mainConfig, "latitude")
	conf.MqttBroker = GetString(configDir, mainConfig, "mqttBroker")
	conf.MqttPort = GetInt(configDir, mainConfig, "mqttPort")
	conf.MqttClientID = GetString(configDir, mainConfig, "mqttClientID")
	conf.LogEvents = mainConfig.Get("logEvents").(bool)

	log.Printf("DEBUG: Main config for %s loaded, MQTT Broker is %s\n", conf.SystemName, conf.MqttBroker)
	log.Printf("DEBUG: Latitude %f, Longitude %f\n", conf.Latitude, conf.Longitude)

	if mainConfig.GetArray("integrations") == nil {
		return conf, errors.New("No Integrations section in config, cannot run")
	}
	conf.Integrations = mainConfig.GetArray("integrations").([]string)
	conf.ConfigDir = configDir
	return conf, nil
}

// GetString retrieves a string value from the config file.
// It looks in secrets.toml or constants.toml if necessary.
func GetString(configDir string, conf *toml.Tree, id string) string {
	raw := conf.Get(id).(string)
	switch raw {
	case secretLabel:
		return getSecretString(configDir, id)
	case constantLabel:
		return getConstantString(configDir, id)
	}
	return raw
}

// GetInt retrieves an int value from the config file.
// It looks in secrets.toml or constants.toml if necessary.
func GetInt(configDir string, conf *toml.Tree, id string) int {
	raw := conf.Get(id)
	if reflect.ValueOf(raw).Kind() == reflect.String {
		switch raw.(string) {
		case secretLabel:
			return getSecretInt(configDir, id)
		case constantLabel:
			return getConstantInt(configDir, id)
		}
	}
	return int(raw.(int64))
}

// GetFloat64 retrieves a float64 value from the config file.
// It looks in secrets.toml or constants.toml if necessary.
func GetFloat64(configDir string, conf *toml.Tree, id string) float64 {
	raw := conf.Get(id)
	if reflect.ValueOf(raw).Kind() == reflect.String {
		switch raw.(string) {
		case secretLabel:
			return getSecretFloat64(configDir, id)
		case constantLabel:
			return getConstantFloat64(configDir, id)
		}
	}
	return raw.(float64)
}

func getSecretString(configDir string, id string) string {
	secretsConfig, err := toml.LoadFile(configDir + secretsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load secrets.toml configuration ", err.Error())
	}
	return secretsConfig.Get(id).(string)
}

func getConstantString(configDir string, id string) string {
	constConfig, err := toml.LoadFile(configDir + constantsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load constants.toml configuration ", err.Error())
	}
	return constConfig.Get(id).(string)
}

func getSecretInt(configDir string, id string) int {
	secretsConfig, err := toml.LoadFile(configDir + secretsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load secrets.toml configuration ", err.Error())
	}
	log.Printf("DEBUG: Looking for %s in secrets file %s\n", id, configDir+secretsFilename)
	return int(secretsConfig.Get(id).(int64))
}

func getConstantInt(configDir string, id string) int {
	constConfig, err := toml.LoadFile(configDir + constantsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load constants.toml configuration ", err.Error())
	}
	return int(constConfig.Get(id).(int64))
}

func getSecretFloat64(configDir string, id string) float64 {
	secretsConfig, err := toml.LoadFile(configDir + secretsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load secrets.toml configuration ", err.Error())
	}
	log.Printf("DEBUG: Looking for %s in secrets file %s\n", id, configDir+secretsFilename)
	return secretsConfig.Get(id).(float64)
}

func getConstantFloat64(configDir string, id string) float64 {
	constConfig, err := toml.LoadFile(configDir + constantsFilename)
	if err != nil {
		log.Panicln("ERROR: Could not load constants.toml configuration ", err.Error())
	}
	return constConfig.Get(id).(float64)
}
