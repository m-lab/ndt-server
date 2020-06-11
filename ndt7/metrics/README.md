# NDT7 Server Metrics

Summary of ndt7 metrics useful for monitoring client requests, measurement
successes, and error rates for the sender and receiver.

* `ndt7_client_connections_total{direction, status}` counts every client
  connection that reaches `handler.Upload` or `handler.Download`.

  * The "direction=" label indicates an "upload" or "download" measurement.
  * The "status=" label is either "result" or a specific error that
    prevented setup before the connection was aborted.
  * All status="result" clients are counted in `ndt7_client_test_results_total`.
  * All status="result" clients should also equal the number of files written.

* `ndt7_client_test_results_total{protocol, direction, result}` counts the
  test results of clients that successfully setup the websocket connection.

  * The "protocol=" label indicates the "ndt7+wss" or "ndt7+ws" protocol.
  * The "direction=" labels are as above.
  * The "result=" label is either "okay-with-rate", "error-with-rate" or
    "error-without-rate".
  * All result=~"*-with-rate" measurements are also recorded in the shared
    test rate histogram.
  * All results are also counted in `ndt7_client_sender_errors_total` and
    `ndt7_client_receiver_errors_total`

* `ndt7_client_sender_errors_total{protocol, direction, error}`
  * The "protocol=" and "direction=" labels are as above.
  * The "error=" label contains unique values mapping to specific error or return
    paths in the sender.

* `ndt7_client_receiver_errors_total{protocol, direction, error}`
  * Just like the `ndt7_client_sender_errors_total` metric, but for the receiver.

Expected invariants:

* `ndt7_client_connections_total{status="result"} == sum(ndt7_client_test_results_total)`
* `sum(ndt7_client_test_results_total) == sum(ndt7_client_sender_errors_total)`
* `sum(ndt7_client_test_results_total) == sum(ndt7_client_receiver_errors_total)`
