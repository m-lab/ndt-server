#!/bin/bash

set -euxo pipefail

openssl genrsa -out test_key.pem
openssl req -new -x509 -key test_key.pem -out test_cert.pem -days 2 -subj "/C=XX/ST=State/L=Locality/O=Org/OU=Unit/CN=localhost/emailAddress=test@email.address"
