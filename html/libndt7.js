/* jshint esversion: 6, asi: true */
/* exported libndt7 */

// libndt7 is a ndt7 client library in JavaScript.
const libndt7 = (function () {
  'use strict'

  // events groups all events
  const events = {
    // open is the event emitted when the socket is opened. The
    // object bound to this event is always null.
    open: 'ndt7.open',

    // close is the event emitted when the socket is closed. The
    // object bound to this event is always null. The code SHOULD
    // always emit this event at the end of the test.
    close: 'ndt7.close',

    // error is the event emitted when the socket is closed. The
    // object bound to this event is always null.
    error: 'ndt7.error',

    // serverMeasurement is a event emitted periodically during a
    // ndt7 download. It represents a measurement performed by the
    // server and sent to us over the WebSocket channel.
    serverMeasurement: 'ndt7.measurement.server',

    // clientMeasurement is a event emitted periodically during a
    // ndt7 download. It represents a measurement performed by the client.
    clientMeasurement: 'ndt7.measurement.client'
  }

  const version = 0.7

  return {
    // version is the client library version.
    version: version,

    // events exports the events table.
    events: events,

    // newClient creates a new ndt7 client with |settings|.
    newClient: function (settings) {
      let funcs = {}

      // emit emits the |value| event identified by |key|.
      const emit = function (key, value) {
        if (funcs.hasOwnProperty(key)) {
          funcs[key](value)
        }
      }

      // makeurl creates the URL from |settings| and |subtest| name.
      const makeurl = function (settings, subtest) {
        let url = new URL(settings.href)
        url.protocol = (url.protocol === 'https:') ? 'wss:' : 'ws:'
        url.pathname = '/ndt/v7/' + subtest
        let params = new URLSearchParams()
        settings.meta = (settings.meta !== undefined) ? settings : {}
        settings.meta['library.name'] = 'libndt7.js'
        settings.meta['library.version'] = version
        for (let key in settings.meta) {
          if (settings.meta.hasOwnProperty(key)) {
            params.append(key, settings.meta[key])
          }
        }
        url.search = params.toString()
        return url.toString()
      }

      // setupconn creates the WebSocket connection and initializes all
      // the event handlers except for the message handler and, when
      // uploading, the connect handler. To setup the WebSocket connection
      // we use the |settings| and the |subtest| arguments.
      const setupconn = function (settings, subtest) {
        const url = makeurl(settings, subtest)
        const socket = new WebSocket(url, 'net.measurementlab.ndt.v7')
        if (subtest === 'download') {
          socket.onopen = function (event) {
            emit(events.open, null)
          }
        }
        socket.onclose = function (event) {
          emit(events.close, null)
        }
        socket.onerror = function (event) {
          emit(events.error, null)
        }
        return socket
      }

      // download measures the download speed using |socket|. To this end, it
      // sets the message handlers of |socket|.
      const download = function (socket) {
        let count = 0
        const t0 = new Date().getTime()
        let tlast = t0
        socket.onmessage = function (event) {
          if (event.data instanceof Blob) {
            count += event.data.size
          } else {
            emit(events.serverMeasurement, JSON.parse(event.data))
            count += event.data.length
          }
          let t1 = new Date().getTime()
          const every = 250  // millisecond
          if (t1 - tlast > every) {
            emit(events.clientMeasurement, {
              elapsed: (t1 - t0) / 1000,  // second
              app_info: {
                num_bytes: count
              }
            })
            tlast = t1
          }
        }
      }

      // uploader performs the read upload.
      const uploader = function (socket, data, t0, tlast, count) {
        let t1 = new Date().getTime()
        const duration = 10000  // millisecond
        if (t1 - t0 > duration) {
          return
        }
        if (socket.bufferedAmount < data.length) {
          socket.send(data)
          count += data.length
        }
        const every = 250  // millisecond
        if (t1 - tlast > every) {
          emit(events.clientMeasurement, {
            elapsed: (t1 - t0) / 1000,  // second
            app_info: {
              num_bytes: count
            }
          })
          tlast = t1
        }
        setTimeout(function() {
          uploader(socket, data, t0, tlast, count)
        }, 0)
      }

      // upload measures the upload speed using |socket|. To this end, it
      // sets the message and open handlers of |socket|.
      const upload = function (socket) {
        socket.onmessage = function (event) {
          emit(events.serverMeasurement, JSON.parse(event.data))
        }
        socket.onopen = function (event) {
          emit(events.open, null)
          const data = new Uint8Array(1 << 13)
          crypto.getRandomValues(data)
          socket.binarytype = 'arraybuffer'
          const t0 = new Date().getTime()
          const tlast = t0
          uploader(socket, data, t0, tlast, 0)
        }
      }

      return {
        // on is a publicly exported function that allows to set a handler
        // for a specific event emitted by this library. |key| is the handler
        // name. |handler| is a callable function.
        on: function (key, handler) {
          funcs[key] = handler
        },

        // startDownload starts the ndt7 download.
        startDownload: function () {
          download(setupconn(settings, 'download'))
        },

        // startUpload starts the ndt7 upload.
        startUpload: function () {
          upload(setupconn(settings, 'upload'))
        }
      }
    }
  }
})()
