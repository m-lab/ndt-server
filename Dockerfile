FROM golang:1.25-alpine3.21 as ndt-server-build
RUN apk add --no-cache git gcc linux-headers musl-dev
ADD . /go/src/github.com/m-lab/ndt-server
RUN /go/src/github.com/m-lab/ndt-server/build.sh

# Now copy the built image into the minimal base image
FROM alpine:3.21
COPY --from=ndt-server-build /go/bin/ndt-server /
COPY --from=ndt-server-build /go/bin/generate-schemas /
ADD ./html /html
WORKDIR /
ENTRYPOINT ["/ndt-server"]
