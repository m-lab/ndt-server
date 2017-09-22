FROM google/cloud-sdk

# Install all the standard packages we need
RUN apt-get -q update
RUN apt-get install -y netcat wget curl openssl sudo man nodejs npm

# Just make the directory for standard logging (though there should be almost none)
RUN mkdir -p /var/spool/iupui_ndt

# NOTE: Start container with --net=host to expose all ports.
# NOTE: Use standard 3010 SSL port.
# NOTE: Web100 variables file must be unnecessary.
# DO NOT ENABLE --snaplog --tcpdump --cputime --adminview -f
# NOTE: this means we log almost nothing.
ADD fake_web100srv.sh /web100srv
CMD /web100srv \
    --tls_port=3010 \
    --private_key=/certs/measurement-lab.org.key \
    --certificate=/certs/measurement-lab.org.crt \
    --log_dir=/var/spool/iupui_ndt \
    --multiple \
    --max_clients=40 \
    --disable_extended_tests
