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
	"testing"
)

func TestGetSubscriberID(t *testing.T) {
	subIDs = make([]string, 20)
	first := GetSubscriberID("test")
	if first != 0 {
		t.Errorf("got %d, expected 0", first)
	}
	second := GetSubscriberID("test")
	if second != 1 {
		t.Errorf("got %d, expected 1", second)
	}
}

func TestSubscription(t *testing.T) {
	subIDs = make([]string, 20)
	subscriptions = make(map[string][]subscriptionT)
	sid := GetSubscriberID("test")
	if isSubscribed(sid, "eventName") {
		t.Error("isSubscribed gave false positive")
	}
	ch, err := Subscribe(sid, "eventName")
	if err != nil {
		t.Errorf(err.Error())
	}
	if ch == nil {
		t.Error("Got nil channel from test subscription")
	}
	if !isSubscribed(sid, "eventName") {
		t.Error("isSubscribed negative for newly-subscribed event")
	}
	ch, err = Subscribe(sid, "eventName")
	if err == nil {
		t.Error("re-subscription to already-subscribed event did not return an error")
	}

	// 2nd sub to same event
	sid2 := GetSubscriberID("test")
	if isSubscribed(sid2, "eventName") {
		t.Error("2nd isSubscribed gave false positive")
	}
	ch, err = Subscribe(sid2, "eventName")
	if err != nil {
		t.Errorf(err.Error())
	}
	if ch == nil {
		t.Error("Got nil channel from 2nd test subscription")
	}
	if !isSubscribed(sid2, "eventName") {
		t.Error("isSubscribed negative for 2nd newly-subscribed event")
	}
	ch, err = Subscribe(sid2, "eventName")
	if err == nil {
		t.Error("2nd re-subscription to already-subscribed event did not return an error")
	}

	// AND unsubscription...
	ch, err = Subscribe(sid, "anotherEventName")
	if err != nil {
		t.Errorf(err.Error())
	}
	if err = Unsubscribe(sid, "eventName"); err != nil {
		t.Error("failed to unsubscribe from event")
	}
	if isSubscribed(sid, "eventName") {
		t.Error("isSubscribed positive for newly-unsubscribed event")
	}
	if !isSubscribed(sid, "anotherEventName") {
		t.Error("isSubscribed negative for previously subscribed event")
	}
}
