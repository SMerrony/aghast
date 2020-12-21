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

package events

import (
	"errors"
	"log"
	"sync"
)

const (
	maxSubscriptions         = 1000
	managerEventsBuffer      = 1000
	subscriberEventsBuffered = 100

	// ActionControlDeviceType must be subscribed to by Integrations providing Action Controls
	ActionControlDeviceType = "Control"
)

// EventT - we keep events as simple as possible.
// Integration/DeviceType/DeviceName identify the sender,
// EventName is only unique to the sender,
// Value is an optional payload
type EventT struct {
	Integration string
	DeviceType  string
	DeviceName  string
	EventName   string
	Value       interface{}
}

type subscriptionT struct {
	subscriber int
	channel    chan EventT
}

var (
	eventMgrChan  chan EventT
	idMu          sync.Mutex
	subIDs        []string
	subsMu        sync.RWMutex
	subscriptions map[string][]subscriptionT
	logEvents     bool
)

// GetSubscriberID returns a subscriber ID which must be used when calling Subscribe or Unsubscribe
func GetSubscriberID(name string) int {
	idMu.Lock()
	if subIDs == nil {
		subIDs = make([]string, maxSubscriptions)
	}
	for i, used := range subIDs {
		if used == "" {
			subIDs[i] = name
			idMu.Unlock()
			return i
		}
	}
	log.Fatalln("ERROR: Exhausted event subscriber IDs")
	return -1
}

// StartEventManager performs any setup required, then launches the eventManager Goroutine.
// It returns the main Event channel to which Integrations should send their Events.
func StartEventManager(logevents bool) chan EventT {
	logEvents = logevents
	eventMgrChan = make(chan EventT, managerEventsBuffer)
	subscriptions = make(map[string][]subscriptionT)
	go eventManager()
	return eventMgrChan
}

func eventManager() {
	for {
		ev := <-eventMgrChan
		if ev.EventName != "Second" && logEvents {
			log.Printf("DEBUG: EventManager got %s event from %s.%s.%s with %v\n", ev.EventName, ev.Integration, ev.DeviceType, ev.DeviceName, ev.Value)
		}
		// TODO Handle system-level events such as 'shutdown'
		subsMu.RLock()
		// explicit subscriptions
		subs, any := subscriptions[getSubKey(ev.Integration, ev.DeviceType, ev.DeviceName, ev.EventName)]
		if any {
			for _, dest := range subs {
				if logEvents {
					log.Printf("DEBUG: ... forwarding event to subscriber %d (%s)\n", dest.subscriber, subIDs[dest.subscriber])
				}
				dest.channel <- ev
			}
		}
		// DeviceName is "+"
		subs, any = subscriptions[getSubKey(ev.Integration, ev.DeviceType, "+", ev.EventName)]
		if any {
			for _, dest := range subs {
				if logEvents {
					log.Printf("DEBUG: ... forwarding event to subscriber %d (%s)\n", dest.subscriber, subIDs[dest.subscriber])
				}
				dest.channel <- ev
			}
		}
		// EventMName is "+"
		subs, any = subscriptions[getSubKey(ev.Integration, ev.DeviceType, ev.DeviceName, "+")]
		if any {
			for _, dest := range subs {
				if logEvents {
					log.Printf("DEBUG: ... forwarding event to subscriber %d (%s)\n", dest.subscriber, subIDs[dest.subscriber])
				}
				dest.channel <- ev
			}
		}
		// DviceName AND EventMName is "+"
		subs, any = subscriptions[getSubKey(ev.Integration, ev.DeviceType, "+", "+")]
		if any {
			for _, dest := range subs {
				if logEvents {
					log.Printf("DEBUG: ... forwarding event to subscriber %d (%s)\n", dest.subscriber, subIDs[dest.subscriber])
				}
				dest.channel <- ev
			}
		}
		subsMu.RUnlock()
	}
}

// Subscribe registers a subscription to an event returning a channel for the events
func Subscribe(subscriberID int, integ, devtyp, devname, evName string) (chan EventT, error) {
	if isSubscribed(subscriberID, integ, devtyp, devname, evName) {
		return nil, errors.New("Already subscribed to event: " + devname + " " + evName)
	}
	newChan := make(chan EventT, subscriberEventsBuffered)
	k := getSubKey(integ, devtyp, devname, evName)
	newSub := subscriptionT{subscriber: subscriberID, channel: newChan}
	subsMu.Lock()
	defer subsMu.Unlock()
	subs, exists := subscriptions[k]
	if !exists {
		ss := make([]subscriptionT, 1)
		ss[0] = newSub
		subscriptions[k] = ss
	} else {
		subs = append(subs, newSub)
	}
	return newChan, nil
}

// Unsubscribe cancels an exisiting event subscription
func Unsubscribe(subscriberID int, integ, devtyp, devname, evName string) error {
	if !isSubscribed(subscriberID, integ, devtyp, devname, evName) {
		return errors.New("Not subscribed to event: " + devname + " " + evName)
	}
	k := getSubKey(integ, devtyp, devname, evName)
	subsMu.Lock()
	defer subsMu.Unlock()
	subs, _ := subscriptions[k]
	var newSubs []subscriptionT
	for _, s := range subs {
		if s.subscriber != subscriberID {
			newSubs = append(newSubs, s)
		}
	}
	subscriptions[k] = newSubs
	return nil
}

func isSubscribed(subscriberID int, integ, devtyp, devname, evName string) bool {
	k := getSubKey(integ, devtyp, devname, evName)
	subsMu.RLock()
	defer subsMu.RUnlock()
	subs, exists := subscriptions[k]
	if !exists {
		return false
	}
	for _, sID := range subs {
		if sID.subscriber == subscriberID {
			return true
		}
	}
	return false
}

func getSubKey(integration, devType, devName, evName string) string {
	return integration + "." + devType + "." + devName + "." + evName
}
