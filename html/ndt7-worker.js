/* jshint esversion: 6, asi: true */
/* globals importScripts, onmessage: true, postMessage, libndt7 */

// ndt7-worker is a Web Worker that runs the ndt7 nettest in a background
// thread of execution, so we do not block the user interface.

importScripts('libndt7.js')

// onmessage receives a key, value message requesting to start a ndt7
// subtest. The key is the subtest type. The value is an object containing
// the settings to be used by the subtest. The worker routes the events
// emitted during the execution of the ndt7 subtest to the main thread using
// again a key, value message. This time the key is the type of event that
// has been emitted and the value is the corresponding object.
onmessage = function (ev) {
  'use strict'
  const msg = ev.data
  if (msg.key === 'download' || msg.key === 'upload') {
    const settings = msg.value
    let clnt = libndt7.newClient(settings)
    for (const key in libndt7.events) {
      if (libndt7.events.hasOwnProperty(key)) {
        clnt.on(libndt7.events[key], function (value) {
          postMessage({
            key: libndt7.events[key],
            value: value,
          })
        })
      }
    }
    if (msg.key === 'download') {
      clnt.startDownload()
    } else {
      clnt.startUpload()
    }
  } else {
    postMessage({
      key: libndt7.events.error,
      value: 'Subtest not implemented: ' + msg.key,
    })
    postMessage({
      key: libndt7.events.close,
      value: null
    })
  }
}
