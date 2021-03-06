# The Daikin Integration

N.B. This code has been independently developed using information freely available on the Internet.  It is not endorsed or supported by Daikin in any way whatsoever.  You use this software at your own risk.

![](../examples/node-red/Screenshots/HVAC-0.0.0.png)

## Description and Purpose
The Daikin Integration provides facilities to control and monitor certain popular Daikin HVAC units.

### Supported Devices
Note the the Integration does not *directly* support specific HVAC units, rather it communicates with the network adapters that may be built-in, or added-to, various 'inverters'.

* BRP069B41 - the Integration has been extensively tested with the BRP069B41 adapters.
We would expect all of the BRP069B41/2/3/4/5 adapters to work.

* BRP069A41/2/3/4/5 - we believe that the previous models, the BRP069A41/2/3/4/5 should also work.

* BRP072A42 - we expect that the BRP072A42 adapter would also work.

We do _not_ expect the BRP072Cnn or SKYFi units to work.

Please let us know if you can add to the above information.

## Configuration
This Integration is enabled by adding a `"daikin",` line to the main `config.toml` file in the "integrations" list. 

A `daikin.toml` file must also exist in your configurations directory.

Its format is...
```
[[Inverter]]
  MAC = "C0E434E69F27"      # The unpunctuated MAC address of the unit
  Label = "Steve's Office"  # A user-friendly label for the unit - must be unique
  
[[Inverter]]
  MAC = "C0E434E6A3C6"
  Label = "Dining Room"

```
We use the MAC address of the interface unit as the IP address or user-assigned unit name (via one of the Daikin apps) could change.

Note that the `label` in the configuration file will form part of the MQTT topic for a specific unit.
Eg. `daikin/Steve's Office/controlinfo` and `daikin/Living Room/temperature`.  Do not include a forward-slash in the label.

## Usage
### Device Discovery
When the Integration is started it scans the local network for Daikin controllers.
Three things can happen...

1. A unit is found which matches one in the `daikin.toml` configuration file (via MAC address). The Integration starts monitoring the unit and it becomes available to the front-end.
2. A unit is found which is not in the configuration file.  It is reported in the AGHAST log and then ignored.
3. One or more units may fail to respond to the scan.  This has been observed to occur from time to time even on well-configured WiFi networks. See below.

Discovery is re-run every 15 minutes - this will pick up any changes, and normally resolves case 3 above.

### Automation
Example Automation...
```
Name = "MorningHeatingStart"
Description = "Main Downstairs Heaters On"
Enabled = true

[Event]
  Name = "Time/Events/TimedEvent/MorningHeatingOn"

[Condition]
   Integration = "Daikin"
   Name = "Steve's Office"
   IsOn = false

[Action.1]
  Integration = "Daikin"
  DeviceLabel = "Hall"
  Execute = [ { Control = "temperature", Setting = 18.0 },
              { Control = "mode",        Setting = "Heat" },
              { Control = "power",       Setting = "on" } ]

# [Action.2]
#   Integration = "Daikin"
#   DeviceLabel = "Living Room"
#   Execute = [ { Control = "temperature", Setting = 18.0 },
#               { Control = "mode",        Setting = "Heat" },
#               { Control = "power",       Setting = "on" } ]  

[Action.3]
  Integration = "Daikin"
  DeviceLabel = "Music Room"
  Execute = [ { Control = "temperature", Setting = 16.0 },
              { Control = "mode",        Setting = "Heat" },
              { Control = "power",       Setting = "on" } ]  

```
Due to the oddities of the Daikin interface there are a couple of things to watch out for in the Actions.
* temperature - must be a float, do not specify an integer
* mode - must be capitalised, one of: "Auto", "Dehumidify", "Cool", "Heat", "Fan"
* fan - this is fan speed, must be quoted as follows...
  * "A" - automatic
  * "B" - silent
  * "3" - level 1
  * ...
  * "7" = level 5
* power - the value must either "on" or "off", YAML booleans will not work

### Queries for Condditions
The Daikin Integration supports AGHAST queries for use in Conditions inside Automations.

Currently supported queries are...
 * `IsAvailable = <true|false>` - tests whether the unit is accessible (online)
 * `IsOn = <true|false>` - tests whether the unit is powered on
  