# Traceroute-caller is the most brittle of our tools, as it requires
# scamper which is not statically linked.  So we work within that image.
FROM measurementlab/traceroute-caller

# UUIDs require a little setup to use appropriately
COPY --from=measurementlab/uuid /create-uuid-prefix-file /
RUN mkdir -p /var/local/uuid/

# tcp-info needs its binary and also needs zstd
COPY --from=measurementlab/tcp-info /usr/local/bin/tcp-info /tcp-info
COPY --from=measurementlab/tcp-info /usr/local/bin/zstd /bin/zstd
COPY --from=measurementlab/tcp-info /usr/local/share/licences/zstd/ /licences/zstd/

# The NDT server needs the server binary and its HTML files
COPY --from=measurementlab/ndt-server /ndt-server /
COPY --from=measurementlab/ndt-server /html /html

RUN mkdir -p /var/spool/ndt

# TODO: Still need packet-headers

WORKDIR /

# You can add further arguments to this when you call the dockerfile.
ENTRYPOINT /create-uuid-prefix-file && \
   mkdir /var/spool/ndt/tcpinfo && \
   /tcp-info \
     --prometheusx.listen-address=:9001 \
     --uuid-prefix-file=/var/local/uuid/prefix \
     & \
   mkdir /var/spool/ndt/traceroute && \
   /traceroute-caller \
     --prometheusx.listen-address=:9002 \
     --uuid-prefix-file=/var/local/uuid/prefix \
     & \
   /ndt-server \
     --uuid-prefix-file=/var/local/uuid/prefix \
     --prometheusx.listen-address=:9003
