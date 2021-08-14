# The mqtt2smtp Integration
## Description and Purpose
Provides a simple MQTT-to-Email gateway for raising alerts etc.

The integration waits for JSON-encoded messages to arrive on the `aghast/mqtt2smtp/send` topic
then tries to send them via the configured email (SMTP) server.

## Configuration
```
# sample configuration for the mqtt2smtp Integration

SmtpHost = "smtp.gmail.com"
SmtpPort = "587"                        # need quotes here
SmtpUser = "!!SECRET(smtpUser)"
SmtpPassword = "!!SECRET(smtpPassword)"
```
## Usage

An example Automation which sends emails every minute - don't do this!

```
Name = "EmailTest"
Description = "Test new mqtt2smtp Integration"
Enabled = true

[Event]
  Topic = "aghast/time/tickers/minutes"

[Action.1]
  Topic = "aghast/mqtt2smtp/send"
  Execute = [ { Key = "To",      Value = "***@*****.***" },
              { Key = "Subject", Value = "Tick-tock" },
              { Key = "Message", Value = "Another minute has passed..."},
            ]
```

The JSON payload must include all three Key/Value pairs:
 - "To": "destination email address"
 - "Subject": "email subject"
 - "Message": "body of email"
