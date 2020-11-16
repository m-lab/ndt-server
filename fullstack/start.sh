#!/bin/bash

# Starts the ndt-server binary running with all its associated supporting
# services set up just right.  If you just want to run a server that speaks the
# unencrypted NDT5 (legacy) protocol, then you can run:
#  $ docker run --net=host measurementlab/ndt
# and you will get an NDT server running on port 3001, with data being saved to
# the in-container directory /var/spool/ndt/
#
# If you would like to run NDT7 tests (which you should, it is a simpler
# protocol and a more robust measurement), then you will need a private key and
# a TLS certificate (let's assume they are called "/etc/certs/key.pem" and
# "/etc/certs/cert.pem").  To run an NDT7 server on port 443, you can do:
#  $ docker run -v /etc/certs:/certs --net=host measurementlab/ndt \
#      --key=/certs/key.pem --cert=/certs/cert.pem
#
# The NDT server produces data on disk. If you would like this data saved to a
# directory outside of the docker container, then you need to mount the external
# directory inside the container at /var/spool/ndt using the -v argument to
# "docker run".
#
# All arguments to this script are passed directly through to ndt-server.

set -euxo pipefail


# Set up the filesystem.

# Set up UUIDs to have a common race-free prefix.
UUID_FILE=$(mktemp /tmp/uuidprefix.XXXXXX)
/create-uuid-prefix-file --filename="${UUID_FILE}"

# Set up the data directory.
DATA_DIR=/var/spool/ndt
mkdir -p "${DATA_DIR}"


# Start all services.

# Start the tcp-info logging service.
mkdir -p "${DATA_DIR}"/tcpinfo
/tcp-info \
  --prometheusx.listen-address=:9991 \
  --uuid-prefix-file="${UUID_FILE}" \
  --output="${DATA_DIR}"/tcpinfo \
  --tcpinfo.eventsocket=/var/local/tcpeventsocket.sock \
  &

while [[ ! -e /var/local/tcpeventsocket.sock ]]; do
  sleep 1
done

# Start the traceroute service.
mkdir -p "${DATA_DIR}"/traceroute
/traceroute-caller \
  --prometheusx.listen-address=:9992 \
  --uuid-prefix-file="${UUID_FILE}" \
  --outputPath="${DATA_DIR}"/traceroute \
  --tcpinfo.eventsocket=/var/local/tcpeventsocket.sock \
  &

# Start packet header capture.
mkdir -p "${DATA_DIR}"/pcap
/packet-headers \
  --prometheusx.listen-address=:9993 \
  --datadir="${DATA_DIR}"/pcap \
  --tcpinfo.eventsocket=/var/local/tcpeventsocket.sock \
  &


# Start the NDT server.
/ndt-server \
 --uuid-prefix-file="${UUID_FILE}" \
 --datadir="${DATA_DIR}" \
 $*
 &

# Any of the backgrounded processes dying should kill the whole thing.
wait -n
