/* jshint esversion: 6, asi: true, worker: true */
// WebWorker that runs the ndt7 download test

// When run by Node.js, we need to import the websocket libraries.
if (typeof WebSocket === 'undefined') {
  global.WebSocket = require('isomorphic-ws');
}

self.onmessage = function (ev) {
  'use strict'
  // TODO put the choce between secure and insecure here
  //let url = new URL(ev.data.href)
  //url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
  //url.pathname = '/ndt/v7/download'
  const url = ev.data['ws:///ndt/v7/download']
  const sock = new WebSocket(url, 'net.measurementlab.ndt.v7')

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
      total += (ev.data.hasOwnProperty('size')) ? ev.data.size : ev.data.length
      // Perform a client-side measurement 4 times per second.
      let now = new Date().getTime()
      const every = 250  // ms
      if (now - previous > every) {
        postMessage({
          MsgType: 'measurement',
          ClientData: {
            ElapsedTime: (now - start) * 1000,  // us
            NumBytes: total,
            MeanClientMbps: total*8 / (now - start) / 1000  // Bytes * 8 bits/byte * 1/(duration in ms) * 1000ms/s * 1 Mb / 1000000 bits = Mb/s
          },
          Source: 'client',
        })
        previous = now
      }

      // Pass along every server-side measurement.
      if (typeof ev.data === 'string') {
        postMessage({
          MsgType: 'measurement',
          ServerMessage: ev.data,
          Source: 'server',
        })
      }
    }
  }
};
