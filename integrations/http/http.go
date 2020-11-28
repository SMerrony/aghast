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

package http

import (
	"bytes"
	"context"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/SMerrony/aghast/events"
	"github.com/SMerrony/aghast/mqtt"
	"github.com/gorilla/mux"
	"github.com/pelletier/go-toml"
)

const (
	configFilename = "/http.toml"
	staticDir      = "/http/static/"
	templateDir    = "/http/templates/"
	timeout        = 5 * time.Second
)

// HTTP is the bundled Integration that provides a browser interface
type HTTP struct {
	srvAddr           string
	siteName          string
	server            *httpServer
	confDir           string
	sseAddr           string
	homepageTpl       *template.Template
	navigationBarHTML string
	timeStr           string
}

type httpServer struct {
	server *http.Server
	wg     sync.WaitGroup
}

// ProvidesDeviceTypes returns a slice of device types that this Integration supplies.
func (h *HTTP) ProvidesDeviceTypes() []string {
	return []string{"HTTP"}
}

// LoadConfig loads and stores the configuration for this Integration
func (h *HTTP) LoadConfig(confdir string) error {
	h.confDir = confdir
	conf, err := toml.LoadFile(confdir + configFilename)
	if err != nil {
		log.Println("ERROR: Could not load configuration ", err.Error())
		return err
	}
	confMap := conf.ToMap()
	h.srvAddr = confMap["address"].(string)
	h.siteName = confMap["siteName"].(string)
	h.sseAddr = confMap["sseAddress"].(string)

	filename := confdir + templateDir + "index.html"
	homePageHTML, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("ERROR: HTTP Could not load %s\n", filename)
		return err
	}
	h.homepageTpl = template.Must(template.New("homepage_view").Parse(string(homePageHTML)))
	filename = confdir + templateDir + "navigation_bar.html"
	navBar, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("ERROR: HTTP Could not load %s\n", filename)
		return err
	}
	h.navigationBarHTML = string(navBar)
	return nil
}

// Start launches the Integration, LoadConfig() should have been called beforehand.
func (h *HTTP) Start(evChan chan events.EventT, mqttChan chan mqtt.MQTTMessageT) {
	sid := events.GetSubscriberID()
	ch, err := events.Subscribe(sid, "Time", "Ticker", "SystemTicker", "Minute")
	if err != nil {
		log.Fatalln("ERROR: HTTP Integration could not subscrible to Ticker event")
	}
	go func() {
		for {
			ev := <-ch
			h.timeStr = ev.Value.(time.Time).Format("15:04")
		}
	}()
	go func() {
		h.startServer()
		defer h.stopServer()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt)
		<-sigChan
	}()
}

func (h *HTTP) startServer() {
	// Setup Context
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup Handlers
	router := mux.NewRouter()
	router.HandleFunc("/", h.homeHandler)
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(h.confDir+staticDir))))

	htmlServer := httpServer{
		server: &http.Server{
			Addr:           h.srvAddr,
			Handler:        router,
			ReadTimeout:    timeout,
			WriteTimeout:   timeout,
			MaxHeaderBytes: 1 << 20,
		},
	}

	// Add to the WaitGroup for the listener goroutine
	htmlServer.wg.Add(1)

	// Start the listener
	go func() {
		log.Printf("DEBUG: HTTP Server : Service started : Host=%s\n", h.srvAddr)
		htmlServer.server.ListenAndServe()
		htmlServer.wg.Done()
	}()

	h.server = &htmlServer
}

func (h *HTTP) stopServer() error {
	// Create a context to attempt a graceful 5 second shutdown.
	const timeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("INFO: HTTP Server : Service stopping\n")

	// Attempt the graceful shutdown by closing the listener
	// and completing all inflight requests
	if err := h.server.server.Shutdown(ctx); err != nil {
		// Looks like we timed out on the graceful shutdown. Force close.
		if err := h.server.server.Close(); err != nil {
			log.Printf("ERROR: HTMLServer : Service stopping : Error=%v\n", err)
			return err
		}
	}

	// Wait for the listener to report that it is closed.
	h.server.wg.Wait()
	log.Printf("INFO: HTTP Server : Stopped\n")
	return nil
}

func (h *HTTP) homeHandler(w http.ResponseWriter, r *http.Request) {
	push(w, h.confDir+staticDir+"style.css")
	push(w, h.confDir+staticDir+"navigation_bar.css")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fullData := map[string]interface{}{
		"NavigationBar": template.HTML(h.navigationBarHTML),
		"SiteName":      h.siteName,
		"Time":          h.timeStr,
		"SSEport":       h.sseAddr,
	}
	render(w, r, h.homepageTpl, "homepage_view", fullData)
}

// push the given resource to the client.
func push(w http.ResponseWriter, resource string) {
	pusher, ok := w.(http.Pusher)
	if ok {
		if err := pusher.Push(resource, nil); err == nil {
			return
		}
	}
}

// render a template, or server error.
func render(w http.ResponseWriter, r *http.Request, tpl *template.Template, name string, data interface{}) {
	buf := new(bytes.Buffer)
	if err := tpl.ExecuteTemplate(buf, name, data); err != nil {
		log.Printf("WARNING: HTTP Render Error: %v\n", err)
		return
	}
	w.Write(buf.Bytes())
}
