go-dsmrp1
---------

Go package to connect to and parse the data send by P1 Smart Meters
used in the Netherlands.

Also included:

1. `dsmrp1d` a daemon that connects to the smart meter
   and makes the latest data available via a simple JSON web service.
2. `dsmrp1-munin` a munin plugin that connects to `dsmrp1d`
