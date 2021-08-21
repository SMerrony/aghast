# Automation

Automations are used to send MQTT messages when certain events occur.

It is expected that Automation facilities will be signficantly expanded, so this page will be updated accordingly.

An Automation may cause one or more Actions to be performed, optionally a Condition
may be checked before running the Actions.

- [Automation](#automation)
  - [Configuration](#configuration)
    - [Preamble](#preamble)
    - [Event](#event)
    - [Condition](#condition)
    - [Actions](#actions)
  - [Examples](#examples)
    - [1. A very simple automation](#1-a-very-simple-automation)
    - [2. Using a value from the triggering event in a condition](#2-using-a-value-from-the-triggering-event-in-a-condition)

## Configuration

Like everything else in AGHAST, Automations are defined in TOML files.
They must be located in an `automation` directory inside your main configuration directory.

Here is an example Automation that turns on a couple of HVAC units every morning...
```
Name        = "MorningHeatingStart"
Description = "Main Downstairs Heaters On"
Enabled     = true
EventTopic  = "aghast/time/events/MorningHeatingOn"

[Action.1]
  Topic     = "daikin2mqtt/Steves_Room/set/controls"
  Payload   = '{"set_temp": 21.0, "mode": "HEAT", "power": true}'

[Action.2]
  Topic     = "daikin2mqtt/Living_Room/set/controls"
  Payload   = '{"set_temp": 19.0, "mode": "HEAT", "power": true}'            
```


We will describe each section below.

### Preamble
 * Name - a unique identifier for this Automation
 * Description
 * Enabled - either `true` or `false`, controls whether the Automation is used or not
 * EventTopic - see below

### Event
Automation processing is triggered by the arrival of an MQTT message we refer to as an 'event'.  
The `EventTopic` line identifies the triggering message.

### Condition
You may optionally specify a Condition that must be satisfied for the Automation to proceed. 
Conditions may refer either to the payload that was delivered with the `EventTopic` message,
or to a separate message received in response to a `QueryTopic` request.

The simplest case is therefore when we want to examine a simple value sent in the original payload...
```
[Condition]
  Is    = ">"    # comparison - one of: "=", "!=", "<", ">", 
  Value = 50.0
```

This is the next simplest case, we query something else and it responds with a single, raw value on the same topic...
```
[Condition]
  QueryTopic = "pizero02/gpio/sensor/dht22_humidity" # No payload or key is required for this query
  Is         = ">"
  Value      = 50.0
```

This target responds with a JSON payload, we need to specify what key to examine...
```
[Condition]
  QueryTopic = "daikin2mqtt/Living_Room/get/sensors" # No payload is required for this query
  ReplyTopic = "daikin2mqtt/Living_Room/sensors"     # But the reply topic is different
  Key        = "ext_temp"
  Is         = "<"
  Value      = 17.0
```

This target needs a custom payload for the query, and returns JSON...
```
[Condition]
  QueryTopic = "zigbee2mqtt/Office_Socket/get"
  ReplyTopic = "zigbee2mqtt/Office_Socket"
  Payload    = '{"state": ""}'
  Key        = "state"
  Is         = "="
  Value      = "ON"
```


Some Integrations supply multiple results (eg. Scraper) and you will need to add an `Index = ` line to the Condition.

There are several comparison operators available for the `Is` clause:
* `"="`  (always use this for boolean true/false values)
* `"!="` (not equal) 
* `"<"`
* `">"`

The retrieved value is compared (i.e. on the left) against the given `Value` (on the right) 

### Actions
One or more Actions must be attached to an Event to form an Automation.

The label `[Action.<label>]` in the Action header is used to sort the actions alphanumerically.

To be usable by Automations, an Integration must accept MQTT 'commands'.

Actions contain two confiurations...
 * Topic - the MQTT address receiving the Control
 * Payload - the MQTT message to be sent

The `Payload` can be either a simple value, or a JSON string.

JSON payloads need to be enclosed either in single-quotes, or be multi-line strings enclosed
in triple-quotes.

## Examples
### 1. A very simple automation
```
Name = "StairwayZapperOff"
Description = "Turn zapper off at sunrise"
Enabled = true
EventTopic = "aghast/time/events/Sunrise"

[Action.1]
  Topic = "zigbee2mqtt/Stairway_Socket/set"
  Payload = '{ "state": "OFF" }'
```
### 2. Using a value from the triggering event in a condition
```
Name        = "ExtraOfficeLampOn"
Description = "Turn on side lamp in office when left wall switch is on"
Enabled     = true
EventTopic  = "zigbee2mqtt/Office_Dual_Switch"

[Condition]
  Key       = "state_left"
  Is        = "="
  Value     = "ON"

[Action.1]
  Topic     = "zigbee2mqtt/Office_3_Way_Socket/set"
  Payload   = '{"state_l3": "ON"}'
```
