/* eslint-env browser, node, worker */

// ndt7 contains the core ndt7 client functionality. Please, refer
// to the ndt7 spec available at the following URL:
//
// https://github.com/m-lab/ndt-server/blob/master/spec/ndt7-protocol.md
//
// This implementation uses v0.9.0 of the spec.

// Wrap everything in a closure to ensure that local definitions don't
// permanently shadow global definitions.
(function() {
  'use strict';

  /**
   * @name ndt7
   * @namespace ndt7
   */
  const ndt7 = (function() {
    const staticMetadata = {
      'client_library_name': 'ndt7-js',
      'client_library_version': '0.0.6',
    };
    // cb creates a default-empty callback function, allowing library users to
    // only need to specify callback functions for the events they care about.
    //
    // This function is not exported.
    const cb = function(name, callbacks, defaultFn) {
      if (typeof(callbacks) !== 'undefined' && name in callbacks) {
        return callbacks[name];
      } else if (typeof defaultFn !== 'undefined') {
        return defaultFn;
      } else {
        // If no default function is provided, use the empty function.
        return function() {};
      }
    };

    // The default response to an error is to throw an exception.
    const defaultErrCallback = function(err) {
      throw new Error(err);
    };

    /**
     * discoverServerURLs contacts a web service (likely the Measurement Lab
     * locate service, but not necessarily) and gets URLs with access tokens in
     * them for the client. It can be short-circuted if config.server exists,
     * which is useful for clients served from the webserver of an NDT server.
     *
     * @param {Object} config - An associative array of configuration options.
     * @param {Object} userCallbacks - An associative array of user callbacks.
     *
     * It uses the callback functions `error`, `serverDiscovery`, and
     * `serverChosen`.
     *
     * @name ndt7.discoverServerURLS
     * @public
     */
    async function discoverServerURLs(config, userCallbacks) {
      config.metadata = Object.assign({}, config.metadata);
      config.metadata = Object.assign(config.metadata, staticMetadata);
      const callbacks = {
        error: cb('error', userCallbacks, defaultErrCallback),
        serverDiscovery: cb('serverDiscovery', userCallbacks),
        serverChosen: cb('serverChosen', userCallbacks),
      };
      let protocol = 'wss';
      if (config && ('protocol' in config)) {
        protocol = config.protocol;
      }

      const metadata = new URLSearchParams(config.metadata);
      // If a server was specified, use it.
      if (config && ('server' in config)) {
        // Add metadata as querystring parameters.
        const downloadURL = new URL(protocol + '://' + config.server + '/ndt/v7/download');
        const uploadURL = new URL(protocol + '://' + config.server + '/ndt/v7/upload');
        downloadURL.search = metadata;
        uploadURL.search = metadata;
        return {
          '///ndt/v7/download': downloadURL.toString(),
          '///ndt/v7/upload': uploadURL.toString(),
        };
      }

      // If no server was specified then use a loadbalancer. If no loadbalancer
      // is specified, use the locate service from Measurement Lab.
      const lbURL = (config && ('loadbalancer' in config)) ? new URL(config.loadbalancer) : new URL('https://locate.measurementlab.net/v2/nearest/ndt/ndt7');
      lbURL.search = metadata;
      callbacks.serverDiscovery({loadbalancer: lbURL});
      const response = await fetch(lbURL).catch((err) => {
        throw new Error(err);
      });
      const js = await response.json();
      if (! ('results' in js) ) {
        callbacks.error(`Could not understand response from ${lbURL}: ${js}`);
        return {};
      }

      // TODO: do not discard unused results. If the first server is unavailable
      // the client should quickly try the next server.
      //
      // Choose the first result sent by the load balancer. This ensures that
      // in cases where we have a single pod in a metro, that pod is used to
      // run the measurement. When there are multiple pods in the same metro,
      // they are randomized by the load balancer already.
      const choice = js.results[0];
      callbacks.serverChosen(choice);

      return {
        '///ndt/v7/download': choice.urls[protocol + ':///ndt/v7/download'],
        '///ndt/v7/upload': choice.urls[protocol + ':///ndt/v7/upload'],
      };
    }

    /*
     * runNDT7Worker is a helper function that runs a webworker. It uses the
     * callback functions `error`, `start`, `measurement`, and `complete`. It
     * returns a c-style return code. 0 is success, non-zero is some kind of
     * failure.
     *
     * @private
     */
    const runNDT7Worker = async function(
        config, callbacks, urlPromise, filename, testType) {
      if (config.userAcceptedDataPolicy !== true &&
          config.mlabDataPolicyInapplicable !== true) {
        callbacks.error('The M-Lab data policy is applicable and the user ' +
                        'has not explicitly accepted that data policy.');
        return 1;
      }

      let clientMeasurement;
      let serverMeasurement;

      // This makes the worker. The worker won't actually start until it
      // receives a message.
      const worker = new Worker(filename);

      // When the workerPromise gets resolved it will terminate the worker.
      // Workers are resolved with c-style return codes. 0 for success,
      // non-zero for failure.
      const workerPromise = new Promise((resolve) => {
        worker.resolve = function(returnCode) {
          if (returnCode == 0) {
            callbacks.complete({
              LastClientMeasurement: clientMeasurement,
              LastServerMeasurement: serverMeasurement,
            });
          }
          worker.terminate();
          resolve(returnCode);
        };
      });

      // If the worker takes 12 seconds, kill it and return an error code.
      // Most clients take longer than 10 seconds to complete the upload and
      // finish sending the buffer's content, sometimes hitting the socket's
      // timeout of 15 seconds. This makes sure uploads terminate on time and
      // get a chance to send one last measurement after 10s.
      const workerTimeout = setTimeout(() => worker.resolve(0), 12000);

      // This is how the worker communicates back to the main thread of
      // execution.  The MsgTpe of `ev` determines which callback the message
      // gets forwarded to.
      worker.onmessage = function(ev) {
        if (!ev.data || !ev.data.MsgType || ev.data.MsgType === 'error') {
          clearTimeout(workerTimeout);
          worker.resolve(1);
          const msg = (!ev.data) ? `${testType} error` : ev.data.Error;
          callbacks.error(msg);
        } else if (ev.data.MsgType === 'start') {
          callbacks.start(ev.data.Data);
        } else if (ev.data.MsgType == 'measurement') {
          // For performance reasons, we parse the JSON outside of the thread
          // doing the downloading or uploading.
          if (ev.data.Source == 'server') {
            serverMeasurement = JSON.parse(ev.data.ServerMessage);
            callbacks.measurement({
              Source: ev.data.Source,
              Data: serverMeasurement,
            });
          } else {
            clientMeasurement = ev.data.ClientData;
            callbacks.measurement({
              Source: ev.data.Source,
              Data: ev.data.ClientData,
            });
          }
        } else if (ev.data.MsgType == 'complete') {
          clearTimeout(workerTimeout);
          worker.resolve(0);
        }
      };

      // We can't start the worker until we know the right server, so we wait
      // here to find that out.
      const urls = await urlPromise.catch((err) => {
        // Clear timer, terminate the worker and rethrow the error.
        clearTimeout(workerTimeout);
        worker.resolve(2);
        throw err;
      });

      // Start the worker.
      worker.postMessage(urls);

      // Await the resolution of the workerPromise.
      return await workerPromise;

      // Liveness guarantee - once the promise is resolved, .terminate() has
      // been called and the webworker will be terminated or in the process of
      // being terminated.
    };

    /**
     * downloadTest runs just the NDT7 download test.
     * @param {Object} config - An associative array of configuration strings
     * @param {Object} userCallbacks
     * @param {Object} urlPromise - A promise that will resolve to urls.
     *
     * @return {number} Zero on success, and non-zero error code on failure.
     *
     * @name ndt7.downloadTest
     * @public
     */
    async function downloadTest(config, userCallbacks, urlPromise) {
      const callbacks = {
        error: cb('error', userCallbacks, defaultErrCallback),
        start: cb('downloadStart', userCallbacks),
        measurement: cb('downloadMeasurement', userCallbacks),
        complete: cb('downloadComplete', userCallbacks),
      };
      const workerfile = config.downloadworkerfile || 'ndt7-download-worker.js';
      return await runNDT7Worker(
          config, callbacks, urlPromise, workerfile, 'download')
          .catch((err) => {
            callbacks.error(err);
          });
    }

    /**
     * uploadTest runs just the NDT7 download test.
     * @param {Object} config - An associative array of configuration strings
     * @param {Object} userCallbacks
     * @param {Object} urlPromise - A promise that will resolve to urls.
     *
     * @return {number} Zero on success, and non-zero error code on failure.
     *
     * @name ndt7.uploadTest
     * @public
     */
    async function uploadTest(config, userCallbacks, urlPromise) {
      const callbacks = {
        error: cb('error', userCallbacks, defaultErrCallback),
        start: cb('uploadStart', userCallbacks),
        measurement: cb('uploadMeasurement', userCallbacks),
        complete: cb('uploadComplete', userCallbacks),
      };
      const workerfile = config.uploadworkerfile || 'ndt7-upload-worker.js';
      const rv = await runNDT7Worker(
          config, callbacks, urlPromise, workerfile, 'upload')
          .catch((err) => {
            callbacks.error(err);
          });
      return rv << 4;
    }

    /**
     * test discovers a server to run against and then runs a download test
     * followed by an upload test.
     *
     * @param {Object} config - An associative array of configuration strings
     * @param {Object} userCallbacks
     *
     * @return {number} Zero on success, and non-zero error code on failure.
     *
     * @name ndt7.test
     * @public
     */
    async function test(config, userCallbacks) {
      // Starts the asynchronous process of server discovery, allowing other
      // stuff to proceed in the background.
      const urlPromise = discoverServerURLs(config, userCallbacks);
      const downloadSuccess = await downloadTest(
          config, userCallbacks, urlPromise);
      const uploadSuccess = await uploadTest(
          config, userCallbacks, urlPromise);
      return downloadSuccess + uploadSuccess;
    }

    return {
      discoverServerURLs: discoverServerURLs,
      downloadTest: downloadTest,
      uploadTest: uploadTest,
      test: test,
    };
  })();

  // Modules are used by `require`, if this file is included on a web page, then
  // module will be undefined and we use the window.ndt7 piece.
  if (typeof module !== 'undefined' && typeof module.exports !== 'undefined') {
    module.exports = ndt7;
  } else {
    window.ndt7 = ndt7;
  }
})();
