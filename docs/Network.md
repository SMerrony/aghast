# The Network Integration
## Description and Purpose
The Network Integration currently provide the HostChecker facility which can monitor the presence and responsiveness of 
other devices on the network.

## Configuration
The HostChecker is configured like this...
```
[HostChecker.MainRouter]
  host = "192.168.1.1"
  label = "4G Router"
  period = 60
  port = 80

[HostChecker.PiHole]
  host = "192.168.1.90"
  label = "Pi-Hole & DHCP"
  period = 60
  port = 80
  ```
There are no defaults and all fields must be provided.
 * [HostChecker.\<name\>] - the name must be unique
 * host - either a quoted IP address or hostname
 * label - a user-friendly label to identify the device
 * period - how often to check the host, in seconds
 * port - the port to test responsiveness on (see below)

The responsiveness is returned as a latency figure in milliseconds, be sure to specify an open port.

## Usage
HostChecker provides state and latency events as AGHAST Events and MQTT messages.

The MQTT messages have these topics: `aghast/hostchecker/<name>/state` and `aghast/hostchecker/<name>/latency`.
The `state` message has a payload of either "true" or "false", i.e. available or unavailable, and the `latency` payload is 
an integer - the number of milliseconds the host took to respond. 

The maximum latency reported is 3000ms; after this period the check times out and the host is considered unresponsive.

