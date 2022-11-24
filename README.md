# JSON API server for MAIKo2 data
## Overview
    This is a simple API server for MAIKo2 data.
    You can retrieve JSON-formatted MAIKo2 raw data by accessing API server.

## Preparation
    - Prepare "raw data index tables (raw events, raw files, planes)" by running make_index in MAIKo2Decoder.
    - Make configuration file (json_server_conf.json), which must be JSON format and composed of the fields below. Refer json_server_conf_sample.json.
        - UserName
            For accessing "raw data index tables" in the DB
        - PassWord
            For accessing "raw data index tables" in the DB
        - RawEventsTable
            Name of "raw events table" in your environment
        - RawFilesTable
            Name of "raw files table" in your environment
        - AllowedOriginsCORS
            Names of the hosts which Cross-Origin Resource Sharing (CORS) is allowed

## Usage
1. Set up server
    $ go run json_server.go
2. Access via port 8080 of the machine on which the server program runs.
    e.g. [IP address of the server]:8080/get/40/1 
        (run40, event1)

## Details
### End point of API
    - /get/:run_id/:event_id
        - You can obtain the "event_id" th event in the raw data file of run "run_id".
### Data format
    - JSON returned from API has 4 fields on its top level.
        - AnodeHit
            Hit information (hit strip, hit clock, time over threshold) of TPC-anode image.
            Variable-length array of array of three unsigned integer (i.e. [3][]uint32).
        - CathodeHit
            Hit information of TPC-cathode image. 
            The format is similar to that of "AnodeHit."
        - AnodeFADC
            Signals information from FADC connected to TPC anode.
            Fixed-length array of signal information (i.e. [CHANNEL_OF_FADC][LENGTH_OF_SIGNAL]uint32).
        - CathodeFADC
            Signals information from FADC connected to TPC cathode.
            The format is similar to that of "AnodeFADC."
    - A hit is expressed in a sequence of three unsigned numbers.
        - [0]: hit strip
        - [1]: hit clock
        - [2]: time over threshold (>=1)