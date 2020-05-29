/**
 * @fileoverview A command-line NDT7 client for node.js that uses the official
 * NDT7 js libraries.
 */

'use strict';

const ndt7 = require('./ndt7');

ndt7.test({
  downloadMeasurement: function() {},
  serverDiscovery: function() {},
  serverChosen: function(server) {
    console.log("Testing to:", {
      machine: server.machine,
      locations: server.location,
    });
  },
  downloadComplete: function(data) {
    console.log("Download test is complete:\n\tInstantaneous server bandwidth: ", data.LastServerMeasurement.BBRInfo.BW * 8 / 1000000, "\n\tMean client bandwidth: ", data.LastClientMeasurement.MeanClientMbps)
  }
}).then(code => {
  process.exit(code);
})
