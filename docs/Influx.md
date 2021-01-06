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
  Integration = "Daikin"
  DeviceDataType = "Inverter"
  DeviceName = "Hall"         
  EventName = "OutsideTemperature"
  DataType = "float"                    # Either 'float', 'integer', or 'string'
  
[[Logger]]
  Name = "HallTemp"
  Integration = "Daikin"
  DeviceDataType = "Inverter"
  DeviceName = "Hall"           
  EventName = "Temperature"
  DataType = "float"
```

## Usage
