# The Influx Integration
## Description and Purpose

The Influx Integration currently provides facilities for logging AGHAST Event data to an [InfluxDB](https://www.influxdata.com/) data store.

## Configuration

```
# InfluxDB connection details
bucket = "aghast"
org = "aghast"
token = "!!SECRET(influxToken)"
url = "http://localhost:8086"

# Data to log
[Logger.OutsideTemp]
  integration = "Daikin"
  deviceType = "Inverter"
  deviceName = "Hall"          
  eventName = "OutsideTemperature"
  type = "float"                    # Either 'float', 'integer', or 'string'
  
```

## Usage
