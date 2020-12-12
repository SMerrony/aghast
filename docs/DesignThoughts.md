# Home Automation System Design Ideas
"A Go Home Automation SysTem" - AGHAST

This document will be updated as ideas are refined and experience is gained with the system.

AGHAST is primarily an automation server - it does not mandate a specific front-end.
All data and controls that should be provided to end-users (i.e. not administrators) are exposed via MQTT.
Node-Red is being used as a front-end and example flows are provided, but other MQTT-connected 
dashboards could be used if prefered.

## Concepts

Here we describe the main ideas that make up the system.

### Integrations

An Integration is all the software that provides support for a type of a concrete or abstract object within the HAS.  An Integration may provide Devices, Events, MQTT topics, etc..

Example Integrations might include...
 * Time
 * Daikin HVAC
 * Web Scraper

Where practical, anything in AGHAST is part of an Integration.  Maybe having 'Time', 'Network' etc. as explicit
integrations (albeit pre-installed) will facilitate easier maintenance of them in the future.

### Devices

A Device is an instance of a device type supported by an Integration.

Example Devices might include...

| Integration | Device Type |
| ----------- | ----------- |
| Network     | Pinger |
| Time        | Clock  |
| Time        | Scheduler |
| Time        | Timer |
| Daikin HVAC | Inverter |


Devices may publish Events, maintain Values that can be queried, and provide Controls.

Every instance of a Device must have a unique name, eg. ModemPinger, SystemClock, SprinklerTimer.

### Events

Integrations and Devices may pubilsh Events to indicate something has happened.

| Integration | Device | Event | Possible Meaning |
| ----------- | ------ | ----- | ------------- |
| Time        | Timer  | Expired | A timer has finished normally |
| Time        | Timer  | Killed | A timer has been aborted |
| Front-End   | -      | Started | An instance of the front-end app has been launched |
| Front-End   | Button | Pushed | A button on the front-end has been clicked |
| Daikin HVAC | HVAC Unit | External Control | A unit has been controlled by some external means |
| Daikin HVAC | HVAC Unit | Temperature Reached | Set temperature achieved |

Events have a source device/integration, event name, and optional Value.

All Events are sent to an 'event manager' which forwards copies of Events to any subscribers.  Integrations, Devices and Automations may all subscribe to Events.  

Events do not persist, and if there are no subscribers for an Event when it is published it 
simply disappears.  Events cannot be queried - that is why Device Values (below) may be requested.

Automations subscribe to Events using the "event" configuration which specifies the source and specific event:
```
event = { from = "<sourceName>", name = "<eventName>"}
```

### Values

Integrations, Devices, and Events may provide Values which indicate the state of something.
Eg. on/off, temperature, brightness.

*There is no separate concept of 'attributes', nor 'sensors'*

### Controls

Integrations and Devices may provide Controls to manipulate the state of something.

### Automations

Automations are used to manipulate controls.  Automations subscribe to a single Event.  
If the Events occurs, the Automation may examine its value, query other Values,
and then manipulate Controls.

Here is an idea of how an Automation could be defined...
```
name = "MorningOfficeWarmup"       # unique name for this Automation
description = "Warm up the office"   

# wait for scheduler to send a halfHourBeforeWork event
event = { from = "scheduler", name = "halfHourBeforeWork"} 

[[conditions]]                      # only if
device = "officeHVAC"               #   officeHVAC device
value = "outsideTemperature"        #     is reporting an outsideTemperature                    
test = ["<", 18]                    #     less than 18

[[actions]]                          # set the
device = "officeHVAC"                #   officeHVAC device
control = "mode"                     #     mode
setting = "heat"                     #     to 'heat'

[[actions]]
device = "officeHVAC"                #   and the officeHVAC device
control = "temperature"              #     temperature  
setting = 20                         #     to 20
```

#### Conditions

If any conditions are specified, they must all be satisfied for the Automation to proceed.
Conditions examine the state of Values.
```
[[conditions]]
device = "<deviceName>"
value = "<valueName>"
test = ["<comparison>", <valueLiteral>]
```
Where `<comparison>` is one of: `"="`, `"!="`, `"<"`, `">"`, `"<="`, `">="`.

#### Actions

Every Automation contains at least one Action.

Initially, we define two types of Action...
 1. Explicit value setting
 2. Invoking a script containing one or more Actions

The first type is shown above in the "MorningOfficeWarmup" Automation, the second type looks like this...
```
[[actions]
script = "turnAllLightsOff.toml"
```
Where the script file is a list of Actions that might be reused in different Automations.


### Configuration

All configuration is to be stored in TOML files - not in the database which is used exclusively for persisting the state of the system and providing history.

The main configuration file `config.toml` will be quite small, containing only some general information about
the system itself, and a list of enabled Integrations, eg.
```
systemName = "Our House"

longitude = 43.5
latitude = 2.0

integrations = [
  "time",
  "network",
  "daikin",
]
```
When the server is started, it is passed only the configuration directory path.  The structure of this directory is well-defined...

  * One master configuration file with high-level info and list of enabled Integrations
  * One configuration file per Integration
  * Optional directories, named after an Integration, containing further configuration
  * A directory of files each containing a single Automation
    * A subdirectory of the above containing any Action scripts

Eg. 
```
ConfigDir/                   # any name - passed to server at startup
  config.toml                # integration configs
  time.toml
  network.toml
  http.toml
  daikin.toml
  automations/               # automations
  |  officeWarmup.toml
  |  scripts/                # scripts shared by automations
  |  |  turnAllLightsOff.toml
  http/                      # further config for the http integration
  |  ...
```

## Design Decisions

  * Client-Server architecture
    * Client could be written in Go using the go-app package to deliver a progressive web app
    * Or, serve up via HTTP and just have plain browser clients
    * Or, server via MQTT and have Node-Red clients :-)
