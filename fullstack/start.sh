#!/bin/bash

# All arguments to this script are passed directly through to ndt-server.

set -euxo pipefail

# Set up UUIDs to have a common race-free prefix.
mkdir -p /var/local/uuid/
/create-uuid-prefix-file --filename=/var/local/uuid/prefix

# Start up the tcp-info logging service.
mkdir -p /var/spool/ndt/tcpinfo
/tcp-info \
  --prometheusx.listen-address=:9001 \
  --uuid-prefix-file=/var/local/uuid/prefix \
  --output=/var/spool/ndt/tcpinfo \
  &

# Start up the traceroute service.
mkdir -p /var/spool/ndt/traceroute
/traceroute-caller \
  --prometheusx.listen-address=:9002 \
  --uuid-prefix-file=/var/local/uuid/prefix \
  --outputPath=/var/spool/ndt/traceroute \
  &

# Start up the NDT server.
/ndt-server \
 --uuid-prefix-file=/var/local/uuid/prefix \
 --datadir=/var/spool/ndt \
 $*
