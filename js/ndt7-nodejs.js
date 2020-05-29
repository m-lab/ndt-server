/**
 * @fileoverview A command-line NDT7 client for node.js that uses the official
 * NDT7 js libraries.
 */

'use strict';

global.fetch = require('node-fetch');
global.Worker = require('webworker-threads').Worker;
global.WebSocket = require('isomorphic-ws');

const ndt7 = require('./ndt7');
ndt7.test();