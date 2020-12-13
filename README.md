# A Go Home Automation SysTem (AGHAST)

AGHAST is primarily an automation _server_ - it does not mandate a specific front-end.
All data and controls that should be provided to end-users (i.e. not administrators) are exposed via MQTT.
Node-Red is being used as a front-end during development and example flows are provided, but other MQTT-connected dashboards could be used if prefered.

We believe that end-users of HA systems are generally not interested in the nuts and bolts, so configuration is performed entirely at the back-end by the in-house geek ;-)

## Requirements

* An MQTT Broker - development is being undertaken using Mosquitto
* An MQTT-Connected dashboard - Node-Red works well and example flows are included

## Integrations

 * Time - Includes: Tickers
 * Network - Includes: HostChecker
 * DataLogger - Log Data to CSV files
 * Daikin - HVAC Control and Monitoring
 * Scraper - Web Scraping

## Configuration

The main configuration file `config.toml` is quite simple, containing only some general information about the system itself, and a list of enabled Integrations, eg.
```
systemName = "Our House"      # Label for the system

longitude = 43.6
latitude = 2.24

mqttBroker = "mediaserver01"  # Hostname or IP of MQTT Broker
mqttPort = 1883               # MQTT Broker port
mqttClientID = "aghast"       # MQTT Client ID

logEvents = false             # A LOT of stuff will be logged if this is true!

# List of Integrations we want enabled
integrations = [
  "time",
  "network",
  "datalogger",
  "daikin",
  "scraper",
]
```

Every Integration _must_ have an associated `[Integration].toml` configuration file in the same directory,
eg. `time.toml`, `daikin.toml`, etc.

N.B. Even if no special configuration is required for an enabled Integration, an empty `[Integration].toml` configuration file must exist.

## Running

The AGHAST server may be started from the command line like this...

`./aghastServer -configdir <path/to/config/dir>`

The `-configdir` argument is compulsory and must refer to a directory containing the configuration files described above.