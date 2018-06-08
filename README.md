# ndt-cloud

To run the server locally, first run `gen_local_test_certs.sh`, and then run the
commands
```bash
docker build -t ndtgo
```
and
```bash
docker run --net=host -v `pwd`:/certs -it -t ndtgo
```

Once you have done that, you should have a server running on port 3010 on
localhost.
