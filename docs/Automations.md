# Automations

Automations are used to manipulate the Controls that various Integrations may provide.

Currently (Dec 2020) all Automations are loaded when AGHAST is started - this will change in a later version.  It is expected that Automation facilities will be signficantly expanded, so this page will be updated accordingly.

## Configuration

Like everything else in AGHAST, Automations are defined in TOML files.
They must be located in an `automations` directory inside your main configuration directory.

Here is an example Automation that turns on a couple of HVAC units every morning...
```
name = "MorningHeatingStart"
description = "Main Downstairs Heaters On"
enabled = true

[event]
  integration = "Time"
  deviceType  = "Events"
  deviceName  = "TimedEvent"
  eventName   = "MorningHeatingOn"

[action.1]
  integration = "Daikin"
  deviceLabel = "Hall"
  execute = [ { control = "temperature", setting = 21.0 },
              { control = "mode",        setting = "Heat" },
              { control = "power",       setting = "on" } ]

[action.2]
  integration = "Daikin"
  deviceLabel = "Living Room"
  execute = [ { control = "temperature", setting = 19.0 },
              { control = "mode",        setting = "Heat" },
              { control = "power",       setting = "on" } ]  
```
We will describe each section below.

### Preamble
 * name - a unique identifier for this Automation
 * description
 * enabled - either `true` or `false`, controls whether the Automation is used or not

### Event
Automation processing is triggered by the arrival of an Event.  Currently only internal events are supported.
 * integration - the Integration sending the Event
 * deviceType - the class of event from the Integration
 * deviceName - the specific source of the Event
 * eventName - the specific event that will trigger the Automation

The "TimedEvent" event source is particularly useful for Automations - see [Time](../integrations/time/time.go) for details.

### Actions
One or more Actions must be attached to an Event to form an Automation.

The label `[action.<label>]` in the Action header is used to sort the actions alphanumerically.

To be usable by Automations, an Integration must provide an (AGHAST-internal) `Control` device type; this will receive control settings and apply them to physical devices.

The first three lines of an Action specify the device to be controlled...
 * integration - the Integration receiving the control
 * deviceType - the type of device receiving the control
 * deviceName - the specific device being controlled

Then follow an `execute` section with one or more lines containing control instructions...
 * execute [ ] - containing by one or more control-setting pairs enclosed in { }s
 * control - the Control provided by the Integration
 * setting - a value to set the Control

Although slightly cumbersome for the simplest case, this syntax allows whole groups of Controls for a specific Device to be set with a short script.
