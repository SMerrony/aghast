# The Daikin Integration

N.B. This code has been independently developed using information freely available on the Internet.  It is not endorsed or supported by Daikin in any way whatsoever.  You use this software at your own risk.

## Description and Purpose
The Daikin Integration provides facilities to control and monitor certain popular Daikin HVAC units.

### Supported Devices
Note the the Integration does not *directly* support particular HVAC units, rather it communicates with the network adapters that may be built-in, or added-to, various 'inverters'.

* BRP069B41 - the Integration has been extensively tested with the BRP069B41 adapters.
We would expect all of the BRP069B41/2/3/4/5 adapters to work.

* BRP069A41/2/3/4/5 - we believe that the previous models, the BRP069A41/2/3/4/5 should also work.

* BRP072A42 - we expect that the BRP072A42 adapter would also work.

We do _not_ expect the BRP072Cnn or SKYFi units to work.

Please let us know if you can add to the above information.

## Configuration
This Integration is enabled by adding a `"daikin",` line to the main `config.toml` file in the "integrations" list. 

A `daikin.toml` file must also exist in you configurations directory.

Its format is...
```
[Inverter.123456789ABC]     # The key is the unpunctuated MAC address of the unit
  label = "Steve's Office"  # A user-friendly label for the unit - must be unique

[Inverter.123456789ABD]   
  label = "Living Room"     
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