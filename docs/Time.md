# The Time Integration
## Description and Purpose
The Time Integration provides two facilities:
 * Regular Tickers
 * User-defined timed Events

The Time Integration must be enabled for AGHAST to function.

User-defined Events are available for Automations and other Integrations to use, eg. to trigger Actions at a specific time.

## Configuration

### System Tickers
These Tickers are defined internally and have no configuration. 

They publish events every new second, minute, hour, and day to...
 * integration - "Time"
 * deviceType - "Ticker"
 * deviceName - "SystemTicker"
 * eventName - one of "Second", "Minute", "Hour", or "Day"

### User-Defined Events
User-defined Events are defined via the `time.toml` file (which must exist even if no Events are defined).

Configuration of User-defined Events is simple...

```
# Example Time configuration
# Times must be double-quoted as "HH:MM:SS"

[[Event]]
  Name = "NightOffPeakStarts"
  Time = "00:50:05"            # Plus 5s to be sure!

[[Event]]
  Name = "NightOffPeakEnds"
  Time = "06:50:00"

```

The Name must be unique and contain no white space.  It is used as the final part of the AGHAST Event address (the 'eventName'), the first three parts are fixed as follows:
 * integration: "Time"
 * deviceType: "Events"
 * deviceName: "TimedEvent"
  
The time must be specified exactly as `"HH:MM:SS"` including the double-quotes (we do not use the TOML time syntax).

## Usage
User-defined Events will normally be used in Automations and possibly also in other Integrations.
They are 'caught' by an `[event]` section in a configuration file.
Eg.
```
[event]
  integration = "Time"
  deviceType  = "Events"
  deviceName  = "TimedEvent"
  eventName   = "NightOffPeakStarts"
```
