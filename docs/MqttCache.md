# The MqttCache Integration
## Description and Purpose
This Integration provides a degree of persistence for transient MQTT messages that are
of interest.

It is particulary useful in the scenario where a MQTT source sends data with zero 
retention and does not provide a method for querying the current data.

For example, some MQTT thermometers might send an update only every minute, or even
only when the temperature changes; using this Integration you can retain and query the
last update on demand.

## WARNING
Unlike most of the AGHAST system, this Integration maintains a live state.  If you are making
heavy use of this, then you might want to consider carefully when to restart AGHAST.

## Configuration
An example should be self-explanatory...
```
# Example MqttCache configuration

[[Buffer]]
  Topic = "pizero01/gpio/sensor/dht22_temperature"
  RetainSecs = 600

[[Buffer]]
  Topic = "pizero02/gpio/sensor/dht22_humidity"
  RetainSecs = 600
```
You may add as many caches as you wish.

## Usage
The MqttCache can be used within AGHAST and by any other MQTT clients on the local network.

### Getting Data
To get the latest data first subscribe to the topic `aghast/mqttcache/<original-topic>`, 
then when you want the data send a `get` request to `aghast/mqttcache/get/<original-topic>`.

#### Full Example
If the data you are interested in originally come from `pizero02/gpio/sensor/dht22_humidity`
then ensure that you have the second Buffer configured as above.  

In your code or front-end do not subscribe to the original source, but rather `aghast/mqttcache/pizero02/gpio/sensor/dht22_humidity`.  

When you want the data send a message to `aghast/mqttcache/get/pizero02/gpio/sensor/dht22_humidity` - the payload is ignored.

MqttCache will respond on the topic `aghast/mqttcache/pizero02/gpio/sensor/dht22_humidity` with the exact payload that was originally sent from the source.

### Error Conditions
There are two possible error conditions:
1. No data have yet been received in a cache
2. The data in a cache has expired

In case 1 you will receive a payload containing `{"Error": "No data collected yet"}`.

In case 2 you will receive a payload containing `{"Error": "Data expired"}`.
