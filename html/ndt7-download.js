/* jshint esversion: 6, asi: true, worker: true */
// WebWorker that runs the ndt7 download test
onmessage = function (ev) {
  'use strict'
  let url = new URL(ev.data.href)
  url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
  url.pathname = '/ndt/v7/download'
  const sock = new WebSocket(url.toString(), 'net.measurementlab.ndt.v7')
  sock.onclose = function () {
    postMessage(null)
  }
  sock.onopen = function () {
    const start = performance.now()
    let previous = start
    let total = 0
    sock.onmessage = function (ev) {
      total += (ev.data instanceof Blob) ? ev.data.size : ev.data.length
      let now = performance.now()
      const every = 250  // ms
      if (now - previous > every) {
        postMessage({
          'AppInfo': {
            'ElapsedTime': (now - start) * 1000,  // us
            'NumBytes': total,
          },
          'Origin': 'client',
          'Test': 'download',
        })
        previous = now
      }
      if (!(ev.data instanceof Blob)) {
        let m = JSON.parse(ev.data)
        m.Origin = 'server'
        m.Test = 'download'
        postMessage(m)
      }
    }
  }
}
