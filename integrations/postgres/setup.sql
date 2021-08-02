-- As postgres user...
-- i.e. su postgres; psql

-- CREATE DATABASE aghast;

-- substitute the user you run AGHAST as for "steve" below...
-- GRANT ALL PRIVILEGES ON DATABASE aghast TO steve;
-- \q

-- Reconnect as "steve" - your AGHAST user...
-- psql aghast

DROP TABLE IF EXISTS logged_integers;
DROP TABLE IF EXISTS logged_floats;
DROP TABLE IF EXISTS logged_strings;
DROP TABLE IF EXISTS names;

CREATE TABLE names
(
    id    SERIAL PRIMARY KEY,
    name  TEXT   NOT NULL,
    topic TEXT   NOT NULL
);
GRANT ALL PRIVILEGES ON names to steve;

CREATE TABLE logged_integers
(
    id INTEGER REFERENCES names(id),
    ts TIMESTAMP WITH TIME ZONE NOT NULL,
    int_val BIGINT NOT NULL,
    CONSTRAINT ints_pkey PRIMARY KEY (id, ts)
);
GRANT ALL PRIVILEGES ON logged_integers to steve;

CREATE TABLE logged_floats
(
    id INTEGER REFERENCES names(id),
    ts TIMESTAMP WITH TIME ZONE NOT NULL,
    float_val FLOAT NOT NULL,
    CONSTRAINT floats_pkey PRIMARY KEY (id, ts)
);
GRANT ALL PRIVILEGES ON logged_floats to steve;

CREATE TABLE logged_strings
(
    id INTEGER REFERENCES names(id),
    ts TIMESTAMP WITH TIME ZONE NOT NULL,
    string_val TEXT NOT NULL,
    CONSTRAINT strings_pkey PRIMARY KEY (id, ts)
);
GRANT ALL PRIVILEGES ON logged_strings to steve;
