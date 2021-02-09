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

package events

import (
	"errors"
	"log"
	"strings"
	"sync"
)

const (
	maxSubscriptions         = 1000
	managerEventsBuffer      = 1000
	subscriberEventsBuffered = 100

	// ActionControlDeviceType must be subscribed to by Integrations providing Action Controls
	ActionControlDeviceType = "Control"

	// QueryDeviceType must be subscribed to by Integration providing data retrieval
	QueryDeviceType = "Query"

	// FetchLast request last value
	FetchLast = "FetchLast"
	// FetchLastIndexed request one of last value set
	FetchLastIndexed = "FetchLastIndexed"
	// IsAvailable request status
	IsAvailable = "IsAvailable"
	// IsOn request status
	IsOn = "IsOn"
)

// Standard - but not compulsory - Event Name elemnts
const (
	EvIntegration = iota
	EvDeviceType
	EvDeviceName
	EvQueryType
	EvIndex
)

// EvControl - Conventional Control element
const EvControl = EvQueryType

// EventT - we keep events as simple as possible.
// Name identifies the event, structured like MQTT preferably using
// the elements enumerated above.
// Value is an optional payload
type EventT struct {
	Name  string
	Value interface{}
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

// DumpSubs is a debugging function...
func DumpSubs() {
	if logEvents {
		log.Println("DEBUG: EventManager Dumping Subscriptions...")
		subsMu.RLock()
		idMu.Lock()
		for e, s := range subscriptions {
			log.Printf("DEBUG: ... Event: %s\n", e)
			for _, sub := range s {
				log.Printf("DEBUG: ... ... %s\n", subIDs[sub.subscriber])
			}
		}
		idMu.Unlock()
		subsMu.RUnlock()
	}
}

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

func sendOrCrash(ev EventT, dest subscriptionT) {
	if logEvents {
		log.Printf("DEBUG: ... forwarding event to subscriber %d (%s)\n", dest.subscriber, subIDs[dest.subscriber])
	}
	select {
	case dest.channel <- ev:
	default:
		log.Fatalf("ERROR: EventManager could not write to subscriber %s channel\n", subIDs[dest.subscriber])
	}
}

// EndsWith returns true if the event name finishes with the arg
func (e *EventT) EndsWith(ending string) bool {
	return strings.HasSuffix(e.Name, "/"+ending)
}

// StartsWith returns true if the event name starts with the arg
func (e *EventT) StartsWith(start string) bool {
	return strings.HasPrefix(e.Name, start+"/")
}

func nameDepth(name string) int {
	return strings.Count(name, "/") + 1
}

func eventManager() {
	for {
		ev := <-eventMgrChan
		// if ev.EventName != "Second" && logEvents {
		if !ev.EndsWith("Second") && logEvents {
			log.Printf("DEBUG: EventManager got %s event with %v\n", ev.Name, ev.Value)
		}
		// TODO Handle system-level events such as 'shutdown'
		subsMu.RLock()

		// explicit subscriptions
		subs, any := subscriptions[ev.Name]
		if any {
			for _, dest := range subs {
				sendOrCrash(ev, dest)
				if logEvents {
					log.Printf("DEBUG: ... forwarding to subscriber No. %d\n", dest.subscriber)
				}
			}
		}

		// match with pluses...
		for key, sub := range subscriptions {
			if strings.Count(key, "+") > 0 && nameDepth(key) == nameDepth(ev.Name) {
				// there are the same number of fields, and some pluses in the subscription
				splitEvent := strings.Split(ev.Name, "/")
				splitSub := strings.Split(key, "/")
				matched := true
				for i := 0; i < len(splitEvent); i++ {
					if splitEvent[i] != splitSub[i] && splitSub[i] != "+" {
						matched = false
						break
					}
				}
				if matched {
					for _, dest := range sub {
						sendOrCrash(ev, dest)
						if logEvents {
							log.Printf("DEBUG: ... forwarding to subscriber No. %d\n", dest.subscriber)
						}
					}
				}
			}
		}

		subsMu.RUnlock()
	}
}

// Subscribe registers a subscription to an event returning a channel for the events
func Subscribe(subscriberID int, evName string) (chan EventT, error) {
	if isSubscribed(subscriberID, evName) {
		return nil, errors.New("Already subscribed to event: " + evName)
	}
	newChan := make(chan EventT, subscriberEventsBuffered)

	newSub := subscriptionT{subscriber: subscriberID, channel: newChan}
	subsMu.Lock()
	defer subsMu.Unlock()
	subs, exists := subscriptions[evName]
	if !exists {
		ss := make([]subscriptionT, 1)
		ss[0] = newSub
		subscriptions[evName] = ss
	} else {
		subs = append(subs, newSub)
		subscriptions[evName] = subs
	}
	if logEvents {
		log.Printf("DEBUG: Event Manager - subscriber No. %d has subscribed to %s\n", subscriberID, evName)
	}
	return newChan, nil
}

// Unsubscribe cancels an exisiting event subscription
func Unsubscribe(subscriberID int, evName string) error {
	if !isSubscribed(subscriberID, evName) {
		return errors.New("Not subscribed to event: " + evName)
	}
	subsMu.Lock()
	defer subsMu.Unlock()
	subs, _ := subscriptions[evName]
	var newSubs []subscriptionT
	for _, s := range subs {
		if s.subscriber != subscriberID {
			newSubs = append(newSubs, s)
		}
	}
	subscriptions[evName] = newSubs
	return nil
}

func isSubscribed(subscriberID int, evName string) bool {
	subsMu.RLock()
	defer subsMu.RUnlock()
	subs, exists := subscriptions[evName]
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
