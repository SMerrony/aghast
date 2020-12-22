# The Influx Integration
## Description and Purpose

The Influx Integration currently provides facilities for logging Event data to an [InfluxDB](https://www.influxdata.com/) data store.

## Configuration

```
# InfluxDB connection details
influxBucket = "aghast"
influxOrg = "aghast"
influxToken = "!!SECRET!!"
influxURL = "http://localhost:8086"

# Data to log
[Logger.OutsideTemp]
  integration = "Daikin"
  deviceType = "Inverter"
  deviceName = "Hall"          # A specific unit
  eventName = "OutsideTemperature"
  type = "float"               # Either 'float', 'integer', or 'string'
```
Note that currently, secrets and constants may only be used for the four connection detail configuration items.

## Usage
