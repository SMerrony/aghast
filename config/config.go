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

package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml"
)

const (
	mainConfigFilename = "/config.toml"
	secretsFilename    = "/secrets.toml"
	constantsFilename  = "/constants.toml"
	secretLabel        = "!!SECRET("
	constantLabel      = "!!CONSTANT("
)

// A MainConfigT holds the top-level configuration details
type MainConfigT struct {
	SystemName          string
	LogEvents           bool
	Longitude, Latitude float64
	MqttBroker          string
	MqttPort            int
	MqttUsername        string
	MqttPassword        string
	MqttClientID        string
	Integrations        []string
	ControlPort         int
	ConfigDir           string
}

// CheckMainConfig performs a simple sanity check on the main config.toml and its directory
func CheckMainConfig(configDir string) error {
	mainConfig, err := toml.LoadFile(configDir + mainConfigFilename)
	if err != nil {
		log.Println("ERROR: Could not load main configuration ", err.Error())
		return err
	}
	systemName := mainConfig.Get("SystemName")
	if systemName == nil {
		return errors.New("No 'SystemName' specified in main configuration")
	}
	log.Printf("INFO: Checking main configuration for '%s'\n", systemName)

	if mainConfig.Get("ControlPort") == nil {
		return errors.New("ControlPort must be specified in main configuration, cannot run")
	}

	if mainConfig.GetArray("Integrations") == nil {
		return errors.New("No Integrations section in config, cannot run")
	}
	integrations := mainConfig.GetArray("Integrations").([]string)
	if len(integrations) == 0 {
		return errors.New("No Integrations enabled, cannot run")
	}
	// there should be a config file for each Integration and the time Integration must be specified
	timeFound := false
	for _, i := range integrations {
		if _, err := os.Stat(configDir + "/" + i + ".toml"); err != nil {
			// or a directory of configs...
			if _, err := os.Stat(configDir + "/" + i); err != nil {
				return errors.New("No config file found for Integration: " + i)
			}
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
	t, err := PreprocessTOML(configDir, mainConfigFilename)
	err = toml.Unmarshal(t, &conf)
	if err != nil {
		log.Fatalf("ERROR: Could not load Main config due to %s\n", err.Error())
		return conf, err
	}
	log.Printf("DEBUG: Main config for %s loaded, MQTT Broker is %s\n", conf.SystemName, conf.MqttBroker)
	conf.ConfigDir = configDir
	return conf, nil
}

// PreprocessTOML reads a TOML config file and substitutes !!SECRET() and !!CONSTANT()
// strings for their corresponding values.
func PreprocessTOML(configDir string, fileName string) (preprocessed []byte, e error) {
	rawFile, err := os.Open(configDir + fileName)
	if err != nil {
		return nil, err
	}
	rawReader := bufio.NewReader(rawFile)

	// preload the secrets and constants configs
	secretsConf, err := toml.LoadFile(configDir + secretsFilename)
	if err != nil {
		log.Println("ERROR: Could not load secrets configuration ", err.Error())
		return nil, err
	}
	constantsConf, err := toml.LoadFile(configDir + constantsFilename)
	if err != nil {
		log.Println("ERROR: Could not load constants configuration ", err.Error())
		return nil, err
	}

	for {
		rawLine, err := rawReader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == io.EOF && len(rawLine) < 3 {
			// log.Printf("DEBUG: ... new TOML file is:\n%s\n", preprocessed)
			return preprocessed, nil
		}
		if sIx := strings.Index(rawLine, secretLabel); sIx != -1 {
			// we have a line like this: port = "!!SECRET(portnum)"
			// log.Printf("DEBUG: Found config line with secret: %s", rawLine)
			procdLine := rawLine[:sIx-1]
			rawLine = rawLine[sIx+len(secretLabel):]
			closingIx := strings.IndexByte(rawLine, ')')
			newName := rawLine[:closingIx]
			// log.Printf("DEBUG: ... substitute name is: %s\n", newName)
			if !secretsConf.Has(newName) {
				return nil, errors.New("Secret not found")
			}
			newVal := secretsConf.Get(newName)
			switch reflect.ValueOf(newVal).Kind() {
			case reflect.String:
				procdLine += "\"" + newVal.(string) + "\"\n"
			case reflect.Int64:
				procdLine += fmt.Sprintf("%d\n", newVal.(int64))
			case reflect.Float64:
				procdLine += fmt.Sprintf("%f\n", newVal.(float64))
			}
			// log.Printf("DEBUG: ... replacement line is: %s", procdLine)
			preprocessed = append(preprocessed, []byte(procdLine)...)
			continue
		}
		if cIx := strings.Index(rawLine, constantLabel); cIx != -1 {
			// we have a line like this: port = "!!CONSTANT(portnum)"
			// log.Printf("DEBUG: Found config line with constant: %s\n", rawLine)
			procdLine := rawLine[:cIx-1]
			rawLine = rawLine[cIx+len(constantLabel):]
			closingIx := strings.IndexByte(rawLine, ')')
			newName := rawLine[:closingIx]
			// log.Printf("DEBUG: ... substitute name is: %s\n", newName)
			if !constantsConf.Has(newName) {
				return nil, errors.New("Constant not found")
			}
			newVal := constantsConf.Get(newName)
			switch reflect.ValueOf(newVal).Kind() {
			case reflect.String:
				procdLine += "\"" + newVal.(string) + "\"\n"
			case reflect.Int64:
				procdLine += fmt.Sprintf("%d\n", newVal.(int64))
			case reflect.Float64:
				procdLine += fmt.Sprintf("%f\n", newVal.(float64))
			}
			// log.Printf("DEBUG: ... replacement line is: %s\n", procdLine)
			preprocessed = append(preprocessed, []byte(procdLine)...)
			continue
		}
		preprocessed = append(preprocessed, []byte(rawLine)...)
	}

	// return preprocessed, nil
}

// ChangeEnabled rewrites an Automation config with the first "Enabled = <bool>" changed to
// the supplied state.
func ChangeEnabled(filepath string, newEnabled bool) (err error) {
	conf, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(conf), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Enabled") {
			if newEnabled {
				lines[i] = "Enabled = true"
			} else {
				lines[i] = "Enabled = false"
			}
			break
		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(filepath, []byte(output), 0644)
	return err
}
