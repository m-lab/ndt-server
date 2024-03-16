This is a docker image that allows people to easily run their own ndt-server
binary, along with some recommended "sidecar" measurement services to grab
richer data.  Advanced users will likely want to configure each service
independently, but this docker image allows interested parties to try running
the server with a simple command:
```
 $ docker run --net=host measurementlab/ndt
```
After that, point your web browser to
[http://localhost:3001/static/widget.html](http://localhost:3001/static/widget.html)
and run a speed test to verify that things work.  The default NDT5 JS client
needs some Javascript client-side optimization (pull requests gratefully
accepted), but you should still be able to run a speed test immediately.

# How to use this image.

If you just want to run a server that speaks the unencrypted NDT5 (legacy)
protocol, then you can run:
```
 $ docker run --net=host measurementlab/ndt
```
and you will get an NDT server running on port 3001, with data being saved to
the in-container directory `/var/spool/ndt/`

If you would like to run NDT7 tests (which you should, it is a simpler protocol
and a more robust measurement) or NDT5 tests over TLS, then you will need a
private key and a TLS certificate (let's assume they are called
`/etc/certs/key.pem` and `/etc/certs/cert.pem`).  To run an NDT7 server on port
443, you must mount the directory with those certificates inside the container,
and then tell the NDT server about those files.  For example:
```
 $ docker run -v /etc/certs:/certs --net=host measurementlab/ndt \
     --key=/certs/key.pem --cert=/certs/cert.pem
```

The NDT server produces data on disk. If you would like this data saved to a
directory outside of the docker container, then you need to mount the external
directory inside the container at `/var/spool/ndt` using the `-v` argument to
`docker run`.

All arguments to `docker run` after the name of the image are passed directly
through to the NDT server.
