# The HostChecker Integration
## Description and Purpose
This Integration provides the HostChecker facility which can monitor the presence and responsiveness of other devices on the network.

## Configuration
The HostChecker is configured like this...
```
[[Checker]]
  Name = "MainRouter"
  Host = "192.168.1.1"
  Label = "4G Router"
  Period = 60
  Port = 80

[[Checker]]
  Name = "PiHole"
  Host = "192.168.1.90"
  Label = "Pi-Hole & DHCP"
  Period = 60
  Port = 80
  ```
There are no defaults and all fields must be provided.
 * Name - the name must be unique
 * Host - either a quoted IP address or hostname
 * Label - a user-friendly label to identify the device
 * Period - how often to check the host, in seconds
 * Port - the port to test responsiveness on (see below)

The responsiveness is returned as a latency figure in milliseconds, be sure to specify an open port.

## Usage
HostChecker provides state and latency events as AGHAST MQTT messages.

The MQTT messages have these topics: `aghast/hostchecker/<Name>/state` and `aghast/hostchecker/<Name>/latency`.
The `state` message has a payload of either "true" or "false", i.e. available or unavailable, and the `latency` payload is 
an integer - the number of milliseconds the host took to respond. 

The maximum latency reported is 2000ms; after this period the check times out and the host is considered unresponsive/unavailable.

### Querying Host Availability via MQTT
Send a `get` request to `aghast/hostchecker/get/<Name>` - where `<Name>` is what you specified in the HostChecker
configuration.

A `state` response will be sent to `aghast/hostchecker/<Name>/state` with a value of either "true" or "false".
