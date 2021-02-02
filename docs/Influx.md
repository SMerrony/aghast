# The Influx Integration
## Description and Purpose

The Influx Integration currently provides facilities for logging AGHAST Event data to an [InfluxDB](https://www.influxdata.com/) data store.

## Configuration

```
# InfluxDB connection details
Bucket = "aghast"
Org = "aghast"
Token = "!!SECRET(influxToken)"
URL = "http://localhost:8086"

# Data to log
[[Logger]]
  Name = "OutsideTemp"      
  EventName = "Daikin/Inverter/Hall/OutsideTemperature"
  DataType = "float"                    # Either 'float', 'integer', or 'string'
  
[[Logger]]
  Name = "HallTemp"   
  EventName = "Daikin/Inverter/Hall/Temperature"
  DataType = "float"
```

## Usage
