# The Influx Integration
## Description and Purpose

The Influx Integration provides facilities for logging MQTT data to an [InfluxDB](https://www.influxdata.com/) data store.

## Configuration

```
# InfluxDB connection details
Bucket = "aghast"
Org = "aghast"
Token = "!!SECRET(influxToken)"
URL = "http://localhost:8086"

# Data to log
[[Logger]]  
  Name = "MusicActualTemp" 
  Topic = "pizero01/gpio/sensor/dht22_temperature"
  DataType = "float"     

[[Logger]]
  Name = "SteveOfficeUnitTemp"       
  Topic = "daikin2mqtt/Steve_Office/sensors"
  Key = "unit_temp"                      # payload is JSON, so must specify key
  DataType = "float"
```

## Usage
You will need to generate an access token in InfluxDB and provide it in the configuration.

Ensure you have created the InfluxDB Bucket nominated in your configuration before starting AGHAST.