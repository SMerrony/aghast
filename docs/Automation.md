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
The  `Topic` line identifies the triggering message.

### Condition
You may optionally specify a Condition that must be satisfied for the Automation to proceed...

Old...
```
[Condition]
  Integration = "PiMqttGpio"
  Name = "MusicRoomTemp"
  Is = "<"                # one of: "=", "!=", "<", ">", 
  Value = 17.0
```

New...


This is the simplest case, the target responds with a single, raw value...
```
[Condition]
  Topic = "pizero02/gpio/sensor/dht22_humidity"  # No payload or key is required for this query
  Is = ">"
  Value = 50.0
```

This target responds with a JSON payload, we need to specify what key to examine...
```
[Condition]
  Topic = "daikin2mqtt/Living_Room/get/sensors"  # No payload is required for this query
  Key = "ext_temp"
  Is = "<"
  Value = 17.0
```

This target needs a custom payload for the query, and returns JSON...
```
[Condition]
  Topic = "zigbee2mqtt/Office_Socket/get"
  Payload = "{\"state\": \"\"}"
  Key = "state"
  Is = "="
  Value = "ON"
```


Some Integrations supply multiple results (eg. Scraper) and you will need to add an `Index = ` line to the Condition.

There are several Condition types:
 * Is - as above, must have comparator and Value specified
 * Index - same as "Is" but for a specified Value from an array of Values
 * IsAvailable - either `true` or `false`
 * IsOn - either `true` or `false`

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
