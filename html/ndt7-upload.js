/* jshint esversion: 6, asi: true, worker: true */
// WebWorker that runs the ndt7 upload test
onmessage = function (ev) {
  'use strict'
  let url = new URL(ev.data.href)
  url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
  const wsproto = 'net.measurementlab.ndt.v7'
  url.pathname = '/ndt/v7/upload'
  const sock = new WebSocket(url.toString(), wsproto)
  sock.onclose = function () {
    postMessage(null)
  }
  function uploader(socket, data, start, previous, total) {
    let now = new Date().getTime()
    const duration = 10000  // millisecond
    if (now - start > duration) {
      sock.close()
      return
    }
    const maxMessageSize = 16777216 /* = (1<<24) = 16MB */
    if (data.length < maxMessageSize && data.length < (total - sock.bufferedAmount)/16) {
      data = new Uint8Array(data.length * 2) // TODO(bassosimone): fill this message
    }
    const underbuffered = 7 * data.length
    while (sock.bufferedAmount < underbuffered) {
      sock.send(data)
      total += data.length
    }
    const every = 250  // millisecond
    if (now - previous > every) {
      postMessage({
        'AppInfo': {
          'ElapsedTime': (now - start) * 1000,  // us
          'NumBytes': (total - sock.bufferedAmount),
        },
        'Origin': 'client',
        'Test': 'upload',
      })
      previous = now
    }
    setTimeout(
      function() { uploader(sock, data, start, previous, total) },
      0)
  }
  sock.onopen = function () {
    const initialMessageSize = 8192 /* (1<<13) */
    const data = new Uint8Array(initialMessageSize) // TODO(bassosimone): fill this message
    sock.binarytype = 'arraybuffer'
    const start = new Date().getTime()
    uploader(sock, data, start, start, 0)
  }
}
