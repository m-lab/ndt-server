FROM golang:1.11 as build
ADD . /go/src/github.com/m-lab/ndt-cloud
RUN /go/src/github.com/m-lab/ndt-cloud/build.sh

# Now copy the built image into the minimal base image
FROM alpine
COPY --from=build /go/bin/ndt-cloud /
ADD ./html /html
WORKDIR /
ENTRYPOINT ["/ndt-cloud"]
