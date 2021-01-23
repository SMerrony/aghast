# The PiMqttGpio Integration
## Description and Purpose
This Integration handles data from https://github.com/flyte/pi-mqtt-gpio .

It adds some value over simply connecting to the Pi directly in a front-end.
Data items defined in this Integration become available in AGHAST and can generate Events if required.
The MQTT data can also be forwarded to appear sourced from AGHAST which may be simpler for front-ends.

Some simple data conversion/cleaning is also provided.

## Configuration
Example...
```
# See https://github.com/flyte/pi-mqtt-gpio for the server

[[Sensor]]
  Name           = "MusicRoomTemp"  # A unique name, will be subtopic if forwarding MQTT
  TopicPrefix    = "pizero01/gpio"  # This must match the topic_prefix setting on the Pi
  SensorType     = "dht22_temperature"
  ValueType      = "float" # One of "string", "integer", or "float"
  RoundToInteger = false
  ForwardEvent   = true    # Do not create an AGHAST Event for each value received   
  ForwardMQTT    = true    # Create an AGHAST-sourced MQTT message for each value

[[Sensor]]
  Name           = "MusicRoomHumidity"
  TopicPrefix    = "pizero01/gpio"  
  SensorType     = "dht22_humidity"
  ValueType      = "float" 
  RoundToInteger = true   # Round a float to nearest integer and then handle as integer
  ForwardAghast  = false  # Do not create an AGHAST Event for each value received   
  ForwardMQTT    = true   # Create an AGHAST-sourced MQTT message for each value
```

For information, the corresponding example Pi-MQTT-GPIO YAML configuration for a remote Pi is...
```
mqtt:
  host: mediaserver01
  port: 1883
  user: ""
  password: ""
  topic_prefix: pizero01/gpio

gpio_modules:
  - name: raspberrypi
    module: raspberrypi

sensor_modules:
  - name: dht22_sensor
    module: dht22
    type: AM2302 # can be  DHT11, DHT22 or AM2302
    pin: 4

sensor_inputs:
  - name: dht22_temperature 
    module: dht22_sensor
    interval: 60 # interval in seconds, that a value is read from the sensor and published
    digits: 4 # number of digits to be round
    type: temperature # Can be temperature or humidity

  - name: dht22_humidity 
    module: dht22_sensor
    interval: 60 # interval in seconds, that a value is read from the sensor and published
    digits: 4 # number of digits to be round
    type: humidity # Can be temperature or humidity   
```

## Usage
