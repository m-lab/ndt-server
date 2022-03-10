# Traceroute-caller is the most brittle of our tools, as it requires
# scamper which is not statically linked.  So we work within that image.
FROM measurementlab/traceroute-caller

# UUIDs require a little setup to use appropriately. In particular, they need a
# unique string written to a well-known location to serve as a prefix.
COPY --from=measurementlab/uuid /create-uuid-prefix-file /

# tcp-info needs its binary and also needs zstd
COPY --from=measurementlab/tcp-info /bin/tcp-info /tcp-info
COPY --from=measurementlab/tcp-info /bin/zstd /bin/zstd
COPY --from=measurementlab/tcp-info /licences/zstd/ /licences/zstd/

# packet-headers needs its binary and libpcap.  There's no good way to get both
# easily from the image, due to C-linking issues and the differences between
# alpine and ubuntu, so just rebuild it here.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y libpcap-dev golang-go git socat
RUN go get github.com/m-lab/packet-headers
RUN mv /root/go/bin/packet-headers /packet-headers

# The NDT server needs the server binary and its HTML files
COPY --from=measurementlab/ndt-server /ndt-server /
COPY --from=measurementlab/ndt-server /html /html

COPY fullstack/start.sh /start.sh
RUN chmod +x /start.sh

WORKDIR /

# You can add further arguments to ndt-server, all the other commands are
# fixed.  Prometheus metrics for tcp-info and traceroute-caller can be
# found on ports 9991 and 9992 (set in the start script), while the ndt
# server metrics can be found on port 9990 by default, but can be set by
# passing --prometheusx.listen-address to that start script.
#
# If you would like to run any SSL/TLS-based tests, you'll need to pass in
# the --cert= and --key= arguments.
ENTRYPOINT ["/start.sh"]
