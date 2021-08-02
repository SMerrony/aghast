# The MqttSender Integration
## Description and Purpose
This Integration provides a means of regularly sending custom MQTT messages.

It is particulary useful in the scenario where an MQTT source does not send regular updates, 
but you do want to regularly update a front-end.

It could also be used instead of an Automation for some simple cases.

## Configuration
```
# Example MqttSender configuration

[[Sender]]
  Topic = "zigbee2mqtt/Office_Socket/get"
  Payload = "{\"state\": \"\"}"     # Use "" if nothing is required
  Interval = "Minutes"              # One of Days, Hours, Minutes, or Seconds
  Period = 1                        # An integral number of "Intervals"
  
[[Sender]]
  Topic = "zigbee2mqtt/Office_3_Way_Socket/get"
  Payload = "{\"state_l1\": \"\"}"
  Interval = "Seconds"              # One of Days, Hours, Minutes, or Seconds
  Period = 90
```
The messages are sent every `Period` `Interval`s.
