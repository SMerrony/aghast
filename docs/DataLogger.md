# The DataLogger Integration
## Description and Purpose
This Integration simply logs the payload (or a JSON value from a payload) of MQTT messages into
CSV files.

## Configuration
An example should be self-explanatory...
```
LogDir = "/tmp"     # Don't use /tmp for real!

[[Logger]]
  LogFile = "musicRoomTemp.csv"
  Topic = "pizero01/gpio/sensor/dht22_temperature"
  FlushEvery = 2       # flush to disk every 2 values

[[Logger]]
  LogFile = "stevesUnitTemp.csv"
  Topic = "daikin2mqtt/Steve_Office/sensors"
  Key = "unit_temp"    # if the payload is JSON, then you must specify a key
  FlushEvery = 2

[[Logger]]
  LogFile = "testTimeLog.csv"
  Topic = "aghast/time/tickers/minutes"
  Key = "minute"
  FlushEvery = 2
  
[[Logger]]
  LogFile = "allTemps.csv"
  Topic = "daikin2mqtt/+/sensors"  # MQTT wildcards are okay
  Key = "unit_temp" 
  FlushEvery = 24
```
You may add as many loggers as you wish.
