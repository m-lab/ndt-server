FROM golang:1.10 as build
ADD . /go/src/github.com/m-lab/ndt-cloud
RUN go get -v github.com/m-lab/ndt-cloud

# Now copy the built image into the minimal base image
FROM gcr.io/distroless/base
COPY --from=build /go/bin/ndt-cloud /
ADD ./html /html
WORKDIR /
CMD ["/ndt-cloud", "--key=/certs/key.pem", "--cert=/certs/cert.pem", "--port=3010"]
