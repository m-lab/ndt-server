# Redis Client for NDT Server

`/ndt-server/redis` creates a wrapper for the public redis package to add methods and functionality (type safety, naming convention, error handling) specific to early termination.

## Testing
There are internal tests for the redis client implementation in `ndt-server/redis` and tests for earlyTermination in `ndt-server/ndt7/handler`.

The tests require a running Redis instance and will gracefully skip if Redis is not available.

To start docker: `docker run -d -p 6379:6379 redis:latest`

To run internal tests: `go test ./redis -v -run Test_SetAndGetTerminationFlag`

To run early termination tests: `go test ./ndt7/handler -v -run Test_checkEarlyTermination`

The functions in hander_test.go should be called as a goroutine during a measurement:
```
ctx, cancel := context.WithCancel(parentCtx)
go h.checkEarlyTermination(ctx, uuid, cancel)
// When flag is set to 1 in Redis, cancel() will be called
```