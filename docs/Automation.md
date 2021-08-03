# Automation

Automations are used to manipulate the Controls that various Integrations may provide.

It is expected that Automation facilities will be signficantly expanded, so this page will be updated accordingly.

## Configuration

Like everything else in AGHAST, Automations are defined in TOML files.
They must be located in an `automation` directory inside your main configuration directory.

Here is an example Automation that turns on a couple of HVAC units every morning...
```
Name = "MorningHeatingStart"
Description = "Main Downstairs Heaters On"
Enabled = true

[Event]
  Topic = "aghast/time/events/NightOffPeakStarts"

[Action.1]
  Topic = "daikin2mqtt/Steves_Room/set/controls"
  Execute = [ { Control = "set_temp", Setting = 21.0 },
              { Control = "mode",        Setting = "HEAT" },
              { Control = "power",       Setting = true } ]

[Action.2]
  Topic = "daikin2mqtt/Living_Room/set/controls"
  Execute = [ { Control = "set_temp",    Setting = 19.0 },
              { Control = "mode",        Setting = "HEAT" },
              { Control = "power",       Setting = true } ]  
```
We will describe each section below.

### Preamble
 * Name - a unique identifier for this Automation
 * Description
 * Enabled - either `true` or `false`, controls whether the Automation is used or not

### Event
Automation processing is triggered by the arrival of an MQTT message we refer to as an 'event'.  
The `Topic` line identifies the triggering message.

### Condition
You may optionally specify a Condition that must be satisfied for the Automation to proceed...

This is the simplest case, the target responds with a single, raw value on the same topic...
```
[Condition]
  QueryTopic = "pizero02/gpio/sensor/dht22_humidity"  # No payload or key is required for this query
  Is = ">"                                            # comparison - one of: "=", "!=", "<", ">", 
  Value = 50.0
```

This target responds with a JSON payload, we need to specify what key to examine...
```
[Condition]
  QueryTopic = "daikin2mqtt/Living_Room/get/sensors"  # No payload is required for this query
  ReplyTopic = "daikin2mqtt/Living_Room/sensors"      # The reply topic is different
  Key = "ext_temp"
  Is = "<"
  Value = 17.0
```

This target needs a custom payload for the query, and returns JSON...
```
[Condition]
  QueryTopic = "zigbee2mqtt/Office_Socket/get"
  ReplyTopic = "zigbee2mqtt/Office_Socket"
  Payload = "{\"state\": \"\"}"
  Key = "state"
  Is = "="
  Value = "ON"
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

To be usable by Automations, an Integration must provide an (AGHAST-internal) `Control` device type; this will receive Control settings and apply them to physical devices.

The first line of an Action specifies the device to be controlled...
 * Topic - the MQTT address receiving the Control

Then follow an `Execute` section with one or more lines containing Control instructions...
 * Execute [ ] - containing by one or more Control-Setting pairs enclosed in { }s
 * Control - the Control provided by the Integration
 * Setting - a value to set the Control

Although slightly cumbersome for the simplest case, this syntax allows whole groups of Controls for a specific Device to be set with a short script.
