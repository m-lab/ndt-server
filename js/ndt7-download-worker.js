/* jshint esversion: 6, asi: true, worker: true */
// WebWorker that runs the ndt7 download test


this.onerror = function (e) { console.log(e); handleException(); }

this.onmessage = function (ev) {
  'use strict'
  console.log("got message:", ev);
  // TODO put the choce between secure and insecure here
  //let url = new URL(ev.data.href)
  //url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
  //url.pathname = '/ndt/v7/download'
  const url = ev.data['ws:///ndt/v7/download']
  console.log("Connecting to " + url)
  return;
  const sock = new WebSocket(url, 'net.measurementlab.ndt.v7')
  console.log("Made websocket object")

  sock.onclose = function () {
    postMessage({
      MsgType: "complete"
    })
  }

  sock.onerror = function (ev) {
    postMessage({
      MsgType: 'error',
      Error: ev,
    })
  }

  sock.onopen = function () {
    const start = new Date().getTime()
    let previous = start
    let total = 0

    sock.onmessage = function (ev) {
      total += (ev.data instanceof Blob) ? ev.data.size : ev.data.length
      
      // Perform a client-side measurement 4 times per second.
      let now = new Date().getTime()
      const every = 250  // ms
      if (now - previous > every) {
        postMessage({
          Data: {
            'ElapsedTime': (now - start) * 1000,  // us
            'NumBytes': total,
          },
          Source: 'client',
        })
        previous = now
      }

      // Pass along every server-side measurement.
      if (!(ev.data instanceof Blob)) {
        let m = JSON.parse(ev.data)
        postMessage({
          Data: m,
          Source: 'server',
        })
      }
    }
  }
};
