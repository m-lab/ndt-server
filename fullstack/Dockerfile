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

# The NDT server needs the server binary and its HTML files
COPY --from=measurementlab/ndt-server /ndt-server /
COPY --from=measurementlab/ndt-server /html /html


# TODO: Still need packet-headers

WORKDIR /

# You can add further arguments to this when you call the dockerfile.
ENTRYPOINT \
   mkdir -p /var/local/uuid/ \
   /create-uuid-prefix-file \
     --filename=/var/local/uuid/prefix \
     && \
   mkdir -p /var/spool/ndt/tcpinfo \
     && \
   /tcp-info \
     --prometheusx.listen-address=:9001 \
     --uuid-prefix-file=/var/local/uuid/prefix \
     --output=/var/spool/ndt/tcpinfo \
     & \
   mkdir -p /var/spool/ndt/traceroute \
     && \
   /traceroute-caller \
     --prometheusx.listen-address=:9002 \
     --uuid-prefix-file=/var/local/uuid/prefix \
     --outputPath=/var/spool/ndt/traceroute \
     & \
   /ndt-server \
     --uuid-prefix-file=/var/local/uuid/prefix \
     --datadir=/var/spool/ndt \
     --prometheusx.listen-address=:9003
