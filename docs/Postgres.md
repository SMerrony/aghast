# The Postgres Integration
## Description and Purpose

The Postgres Integration currently provides facilities for logging AGHAST Event data to
a PostgreSQL database.

It logs, as simply as possible, values from AGHAST Events in one of three tables:
 * logged_integers
 * logged_floats
 * logged_strings

Also, the `names` table indexes the Logger Names provided in the configuration file.  It is automatically populated.

## Configuration
Consult the provided `setup.sql` for configuring your DB before using this Integration.

Example `postgres.toml` file...

```
# Postgres connection details
PgHost = "localhost"
PgPort = "5432"        # Use quotes for this
PgUser = "steve"
PgPassword = "aghast"  # Use a !!SECRET in production
PgDatabase = "aghast"

[[Logger]]  
  Name = "OutsideUnitTemp"      
  EventName = "Daikin/Inverter/Hall/OutsideTemperature"
  DataType = "float"                    # Either 'float', 'integer', or 'string'
  
[[Logger]]
  Name = "MusicUnitTemp"       
  EventName = "Daikin/Inverter/Music Room/Temperature"
  DataType = "float"
```
Note that the `EventName`s are AGHAST Events, not MQTT messages.

## Usage
