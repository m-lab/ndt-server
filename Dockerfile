FROM golang:1.11 as build
ADD . /go/src/github.com/m-lab/ndt-server
RUN /go/src/github.com/m-lab/ndt-server/build.sh

# Now copy the built image into the minimal base image
FROM alpine
COPY --from=build /go/bin/ndt-server /
ADD ./html /html
WORKDIR /
ENTRYPOINT ["/ndt-server"]
