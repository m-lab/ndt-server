/* jshint esversion: 6, asi: true, worker: true */
// WebWorker that runs the ndt7 ping test
onmessage = function (ev) {
  'use strict'
  let url = new URL(ev.data.href)
  url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
  url.pathname = '/ndt/v7/ping'
  const sock = new WebSocket(url.toString(), 'net.measurementlab.ndt.v7')
  sock.onclose = function () {
    postMessage(null)
  }
  sock.onopen = function () {
    sock.onmessage = function (ev) {
      if (!(ev.data instanceof Blob)) {
        let m = JSON.parse(ev.data)
        m.Origin = 'server'
        m.Test = 'ping'
        postMessage(m)
      }
    }
  }
}
