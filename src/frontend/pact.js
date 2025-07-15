// pact.js
const path = require('path');

module.exports = {
  consumer: {
    name: 'Frontend'
  },
  provider: {
    name: 'ShippingService'
  },
  pactFolder: path.resolve(process.cwd(), 'pacts'),
  log: path.resolve(process.cwd(), 'logs', 'pact.log'),
  logLevel: 'info',
  spec: 3
};