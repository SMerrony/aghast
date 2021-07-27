# A Go Home Automation SysTem (AGHAST)

AGHAST is primarily an MQTT-centric automation _server_ - it does not mandate a specific front-end.
All data and controls that should be provided to end-users (i.e. not administrators) are exposed via MQTT.
Node-Red is being used as a front-end during development and example flows are provided, but other MQTT-connected dashboards could be used if prefered.

![](examples/node-red/Screenshots/Network-0.0.0.png)

We believe that end-users of HA systems are generally not interested in the nuts and bolts, so configuration is performed entirely at the back-end by the in-house geek ;-)

## Requirements

* An MQTT Broker - development is being undertaken using Mosquitto
* An MQTT-Connected dashboard - Node-Red works well and example flows are included

## Integrations
Integrations provide support for interacting with real-world or virtual resources, eg. Wifi lights, web scrapers, HVAC systems.

Currently available Integrations...
| Integration | Description                  | Documentation |
| ----------- | :--------------------------  |  ------------- |
| Time        | Includes: Tickers                | [Time](docs/Time.md) |
| Automation  | Event-based Automation           | [Automation](docs/Automation.md) |
| DataLogger  | Log Data to CSV files            | [](docs/) |
| ~~Daikin~~  | ~~HVAC Control and Monitoring~~  | *Use [daikin2mqtt](https://github.com/SMerrony/daikin2mqtt) instead* |
| HostChecker | Monitor Device availability      | [HostChecker](docs/HostChecker.md) |
| Influx      | Log Data to InfluxDB             | [Influx](docs/Influx.md) |
| PiMqttGpio  | Capture pi-mqtt-gpio data        | [PiMqttGpio](docs/PiMqttGpio.md) |
| Postgres    | Log Data to PostgreSQL DB        | [Postgres](docs/Postgres.md) |
| Scraper     | Web Scraping                     | [Scraper](docs/Scraper.md) |
| Tuya        | Tuya WiFi lights, ZigBee Sockets | Deprecated [](docs/) |
| ~~Zigbee2MQTT~~ | ~~Zigbee2MQTT sockets...~~   | *Not required with new builtin MQTT functionality* |

The Time Integration must be enabled for AGHAST to start, you will also probably need to
enable Automation and at least one other Integration in order to do anything useful.

The Tuya Integration is a bit of a hack.  But... it can be used to integrated LIDL SmartHome ZigBee 
(and other ZigBee stuff) if they are first added to the TuyaSmart app.

## Configuration

The main configuration file `config.toml` is quite simple, containing only some general information about the system itself, and a list of enabled Integrations, eg.
```
SystemName = "Our House"      # Label for the system
Postcode = "!!SECRET(postcode)"

MqttBroker = "!!CONSTANT(mqttBroker)"  # Hostname or IP of MQTT Broker
MqttPort = 1883               # MQTT Broker port
MqttClientID = "aghast"       # MQTT Client ID
MqttBaseTopic = "aghast"      # First element of topic for all messages we send

LogEvents = false             # A LOT of stuff will be logged if this is true!

ControlPort = 46445           # HTTP port for back-end admin control

# List of Integrations we want enabled
Integrations = [
  "time",         # the Time integration MUST be enabled
  "automation",
#  "datalogger",  # Commented out, will not be enabled
  "hostchecker",
  "influx",
  "pimqttgpio",
  "postgres",
  "scraper",
#  "tuya",
]
```
Every Integration **must** have an associated `<Integration>.toml` configuration file or `<Integration>` subdirectory in the same directory,
eg. `time.toml`, `datalogger.toml`, `automation`, etc.

Even if no special configuration is required for an enabled Integration, an empty `<Integration>.toml` configuration file or `<Integration>` subdirectory must exist.

### Secrets and Constants

You may replace a **value** that you don't want to share with the special string `"!!SECRET(name)"` (even if it is a number).
AGHAST will then look for the matching name-value pair in the `secrets.toml` file and substitute the value found.

Similarly, you can replace a **value** with `"!!CONSTANT(name)"` and it will be fetched from the `constants.toml` file.
This could be especially useful in Automations, where values might be reused several times.

Currently, secrets and constants are supported for string, integer and floating-point values.

## Running

The AGHAST server may be started from the command line like this...

`./aghastServer -configdir <path/to/config/dir>`

The `-configdir` argument is compulsory and must refer to a directory containing the configuration files described above.

AGHAST is largely stateless (unless Integrations explicitly hold some state), it may be started and stopped without
losing any data.  There is no intrinsic requirement for a database for the AGHAST core system.

## MQTT Aide-Memoire
From time-to-time you may wish to manually purge any retained MQTT messages...
`mqtt-forget -t '+/#' -f`
