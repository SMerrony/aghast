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
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/SMerrony/aghast/config"
	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/postgres.toml"
	subscribeName  = "Postgres"
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
}

type loggerT struct {
	Name      string
	EventName string
	DataType  string
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

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (p *Postgres) ProvidesDeviceTypes() []string {
	return []string{"Logger"}
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (p *Postgres) Start(evChan chan events.EventT, mq mqtt.MQTT) {
	p.mutex.Lock()
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

func (p *Postgres) addStopChan() (ix int) {
	p.mutex.Lock()
	p.stopChans = append(p.stopChans, make(chan bool))
	ix = len(p.stopChans) - 1
	p.mutex.Unlock()
	return ix
}

func (p *Postgres) logger(l loggerT) {
	sid := events.GetSubscriberID(subscribeName)
	ch, err := events.Subscribe(sid, l.EventName)
	if err != nil {
		log.Printf("WARNING: Postgres Integration (logger) could not subscribe to event for %v\n", l)
		return
	}
	// lookup or create id value for this data name
	sql := "SELECT id FROM names WHERE name = '" + l.Name + "'"
	var nameID int
	err = p.dbpool.QueryRow(context.Background(), sql).Scan(&nameID)
	if err == pgx.ErrNoRows {
		sql = "INSERT INTO names(id, name) VALUES(DEFAULT, '" + l.Name + "')"
		_, err = p.dbpool.Exec(context.Background(), sql)
		if err != nil {
			log.Println("WARNING: Postgres Integration could not insert into 'names' table")
			return
		}
		sql := "SELECT id FROM names WHERE name = '" + l.Name + "'"
		err = p.dbpool.QueryRow(context.Background(), sql).Scan(&nameID)
		if err != nil {
			log.Println("WARNING: Postgres Integration could not SELECT from  'names' table")
			return
		}
	} else {
		if err != nil {
			log.Println("WARNING: Postgres Integration could not query 'names' table")
			return
		}
	}
	idString := strconv.Itoa(nameID)
	sc := p.addStopChan()
	p.mutex.RLock()
	stopChan := p.stopChans[sc]
	p.mutex.RUnlock()
	log.Printf("DEBUG: Postgres logger starting for %s, subscriber #: %d\n", l.EventName, sid)
	for {
		select {
		case <-stopChan:
			return
		case ev := <-ch:
			switch l.DataType {
			case "float":
				var fl float64
				switch ev.Value.(type) {
				case float32:
					fl = float64(ev.Value.(float32))
				case float64:
					fl = ev.Value.(float64)
				case string:
					fl, err = strconv.ParseFloat(ev.Value.(string), 64)
					if err != nil {
						log.Printf("WARNING: Postgres logger could not parse float from %v\n", ev.Value.(string))
						continue
					}
				}
				sql = fmt.Sprintf("INSERT INTO logged_floats(id, ts, float_val) VALUES( %s, NOW(), %f)", idString, fl)
			case "integer":
				var num int
				switch ev.Value.(type) {
				case int:
					num = ev.Value.(int)
				case int32:
					num = int(ev.Value.(int32))
				case int64:
					num = int(ev.Value.(int64))
				case string:
					num, err = strconv.Atoi(ev.Value.(string))
				}
				if err != nil {
					log.Printf("WARNING: Postgres logger could not parse integer from %v\n", ev.Value.(string))
					continue
				}
				sql = fmt.Sprintf("INSERT INTO logged_integers(id, ts, int_val) VALUES( %s, NOW(), %d)", idString, num)
			case "string":
				sql = fmt.Sprintf("INSERT INTO logged_strings(id, ts, string_val) VALUES( %s, NOW(), %s)", idString, ev.Value.(string))
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
