/* jshint esversion: 6, asi: true */
// ndt7core is a simple ndt7 client API.
const ndt7core = (function() {
  return {
    // run runs the specified test with the specified base URL and calls
    // callback to notify the caller of ndt7 events.
    run: function(baseURL, testName, callback) {
      callback('starting', {Origin: 'client', Test: testName})
      let done = false
      let worker = new Worker('ndt7-' + testName + '.js')
      function finish() {
        if (!done) {
          done = true
          if (callback !== undefined) {
            callback('complete', {Origin: 'client', Test: testName})
          }
        }
      }
      worker.onmessage = function (ev) {
        if (ev.data === null) {
          finish()
          return
        }
        callback('measurement', ev.data)
      }
      // Kill the worker after the timeout. This force the browser to
      // close the WebSockets and prevent too-long tests.
      setTimeout(function () {
        worker.terminate()
        finish()
      }, 10000)
      worker.postMessage({
        href: baseURL,
      })
    }
  }
}())
