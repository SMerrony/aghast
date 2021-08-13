// Copyright ©2021 Steve Merrony

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

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/pelletier/go-toml"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/mqtt"
)

const (
	configFilename = "/postgres.toml"
)

// The Postgres type encapsulates the Postgres Data Logging Integration
type Postgres struct {
	PgHost     string
	PgPort     string
	PgUser     string
	PgPassword string
	PgDatabase string
	Logger     []loggerT
	mutex      sync.RWMutex
	stopChans  []chan bool // used for stopping Goroutines
	dbpool     *pgxpool.Pool
	mq         *mqtt.MQTT
}

type loggerT struct {
	Name     string
	Topic    string
	Key      string
	DataType string
}

// LoadConfig loads and stores the configuration for this Integration
func (p *Postgres) LoadConfig(confdir string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	confBytes, err := config.PreprocessTOML(confdir, configFilename)
	if err != nil {
		log.Println("ERROR: Could not load Postgres configuration ", err.Error())
		return err
	}
	err = toml.Unmarshal(confBytes, p)
	if err != nil {
		log.Fatalf("ERROR: Could not load Postgres config due to %s\n", err.Error())
		return err
	}
	log.Printf("INFO: Postgres has %d loggers\n", len(p.Logger))
	return nil
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (p *Postgres) Start(mq *mqtt.MQTT) {
	p.mutex.Lock()
	p.mq = mq
	var err error
	dbURL := "postgresql://" + p.PgUser + ":" + p.PgPassword + "@" + p.PgHost + ":" + p.PgPort + "/" + p.PgDatabase
	p.dbpool, err = pgxpool.Connect(context.Background(), dbURL)
	if err != nil {
		log.Printf("WARNING: Postgres Integration failed to connect to DB with %s - %s\n", dbURL, err.Error())
		return
	}
	p.mutex.Unlock()
	for _, l := range p.Logger {
		go p.logger(l)
	}
}

// Stop terminates the Integration and all Goroutines it contains
func (p *Postgres) Stop() {
	for _, ch := range p.stopChans {
		ch <- true
	}
	p.dbpool.Close()
	log.Println("DEBUG: Postgres - All Goroutines should have stopped")
}

func (p *Postgres) addStopChan() chan bool {
	newChan := make(chan bool)
	p.mutex.Lock()
	p.stopChans = append(p.stopChans, newChan)
	p.mutex.Unlock()
	return newChan
}

func (p *Postgres) logger(l loggerT) {
	ch := p.mq.SubscribeToTopic(l.Topic)
	defer p.mq.UnsubscribeFromTopic(l.Topic, ch)

	// lookup or create id value for this data name
	sql := "SELECT id FROM names WHERE name = '" + l.Name + "'"
	var nameID int
	err := p.dbpool.QueryRow(context.Background(), sql).Scan(&nameID)
	if err == pgx.ErrNoRows {
		sql = "INSERT INTO names(id, name, topic) VALUES(DEFAULT, '" + l.Name + "', '" + l.Topic + "')"
		_, err = p.dbpool.Exec(context.Background(), sql)
		if err != nil {
			log.Printf("ERROR: Postgres Integration could not insert into 'names' table with query\n%s\n", sql)
			return
		}
		sql := "SELECT id FROM names WHERE name = '" + l.Name + "'"
		err = p.dbpool.QueryRow(context.Background(), sql).Scan(&nameID)
		if err != nil {
			log.Println("ERROR: Postgres Integration could not SELECT from  'names' table")
			return
		}
	} else {
		if err != nil {
			log.Println("ERROR: Postgres Integration could not query 'names' table")
			return
		}
	}
	idString := strconv.Itoa(nameID)
	stopChan := p.addStopChan()
	log.Printf("DEBUG: Postgres logger starting for %s\n", l.Topic)
	for {
		select {
		case <-stopChan:
			return
		case msg := <-ch:
			var value interface{}
			if l.Key == "" {
				value = string(msg.Payload.([]uint8))
			} else {
				jsonMap := make(map[string]interface{})
				err := json.Unmarshal([]byte(msg.Payload.([]uint8)), &jsonMap)
				if err != nil {
					log.Printf("ERROR: DataLogger - Could not understand JSON %s\n", msg.Payload.(string))
					return
				}
				v, found := jsonMap[l.Key]
				if !found {
					log.Printf("ERROR: DataLogger - Could find Key in JSON %s\n", msg.Payload.(string))
					return
				}
				value = v
			}
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
						log.Printf("WARNING: Postgres logger could not parse float from %v\n", value.(string))
						continue
					}
				}
				sql = fmt.Sprintf("INSERT INTO logged_floats(id, ts, float_val) VALUES( %s, NOW(), %f)", idString, fl)
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
					log.Printf("WARNING: Postgres logger could not parse integer from %v\n", value.(string))
					continue
				}
				sql = fmt.Sprintf("INSERT INTO logged_integers(id, ts, int_val) VALUES( %s, NOW(), %d)", idString, num)
			case "string":
				sql = fmt.Sprintf("INSERT INTO logged_strings(id, ts, string_val) VALUES( %s, NOW(), %s)", idString, value.(string))
			default:
				log.Printf("WARNING: Postgres unrecognised ValueType: %s\n", l.DataType)
				continue
			}
			_, err = p.dbpool.Exec(context.Background(), sql)
			if err != nil {
				log.Printf("WARNING: Postgres Integration could not INSERT value - %s\n", err.Error())
			}
		}
	}
}
