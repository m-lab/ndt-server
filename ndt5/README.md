# NDT5 ndt-server code

All code in this directory tree is related to the support of the legacy NDT5
protocol. We have many extant clients that use this protocol, and we don't
want to leave them high and dry, but new clients are encouraged to use the
services provided by ndt7. The test is streamlined, the client is easier to
write, and basically everything about it is better.

In this subtree, we support existing clients, but we will be adding no new
functionality. If you are reading this and trying to decide how to implement
a speed test, use ndt7 and not the legacy, ndt5 protocol. The legacy protocol is
deprecated. It will be supported until usage drops to very low levels, but it
is also not recommended for new integrations or code.

## NDT5 Metrics

Summary of metrics useful for monitoring client request, success, and error rates.

* `ndt5_control_total{protocol, result}` counts every client connection
  that reaches `HandleControlChannel`.

  * The "protocol=" label matches the client protocol, e.g., "WS", "WSS", or
    "PLAIN".
  * The "result=" label is either "okay" or "panic".
  * All result="panic" results also count specific causes in
    `ndt5_control_panic_total`.
  * All result="okay" results come from "protocol complete" clients.

* `ndt5_client_test_requested_total{protocol, direction}` counts
  client-requested tests.

  * The "protocol=" label is the same as above.
  * The "direction=" label will have values like "c2s" and "s2c".
  * If the client continues the test, then the result will be counted in
    `ndt5_client_test_results_total`.

* `ndt5_client_test_results_total{protocol, direction, result}` counts the
  results of client-requested tests.

  * The "protocol=" and "direction=" labels are as above.
  * The "result=" label is either "okay-with-rate", "error-with-rate" or
    "error-without-rate".
  * All result="okay-with-rate" count all "protocol complete" clients up to that
    point.
  * All result=~"error-.*" results also count specific causes in
    `ndt5_client_test_errors_total`.

* `ndt5_client_test_errors_total{protocol, direction, error}`

  * The "protocol=" and "direction=" labels are as above.
  * The "error=" label contains unique values mapping to specific error paths in
    the ndt-server.

Expected invariants:

* `sum(ndt5_control_channel_duration_count) == sum(ndt5_control_total)`
* `sum(ndt5_control_total{result="panic"}) == sum(ndt5_control_panic_total)`
* `sum(ndt5_client_test_results_total{result=~"error.*"}) == sum(ndt5_client_test_errors_total)`

NOTE:

* `ndt5_client_test_results_total` may be less than `ndt5_client_test_requested_total`
  if the client hangs up before the test can run.
