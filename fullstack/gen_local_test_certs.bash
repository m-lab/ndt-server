#!/bin/bash

set -euxo pipefail

openssl genrsa -out key.pem
openssl req -new -x509 -key key.pem -out cert.pem -days 2 -subj "/C=XX/ST=State/L=Locality/O=Org/OU=Unit/CN=localhost/emailAddress=test@email.address"
mv key.pem cert.pem certs/
