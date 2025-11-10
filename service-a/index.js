const express = require('express');
const client = require('prom-client');
const app = express();
const PORT = 3000;

// 1. Create a registry
const register = new client.Registry();
register.setDefaultLabels({
  app: 'service-a'
});
client.collectDefaultMetrics({ register });

// 2. Create custom metrics
const httpRequestCounter = new client.Counter({
  name: 'http_requests_total',
  help: 'Total number of HTTP requests',
  labelNames: ['method', 'route', 'status_code'],
  registers: [register],
});

const httpRequestDuration = new client.Histogram({
  name: 'http_request_duration_seconds',
  help: 'Duration of HTTP requests in seconds',
  labelNames: ['method', 'route'],
  buckets: [0.1, 0.5, 1, 1.5, 2, 5], // Buckets for latency
  registers: [register],
});

// 3. Instrument the main endpoint
app.get('/', (req, res) => {
  console.log('Request received for service-a');
  
  // Start the timer
  const end = httpRequestDuration.startTimer({
    method: req.method,
    route: req.path,
  });
  var count = 0;
  while(count<100000000) count++;

  // Send the response
  res.json({
    service: "service-a",
    status: "ok"
  });

  // Stop timer and record success
  end();
  httpRequestCounter.inc({
    method: req.method,
    route: req.path,
    status_code: res.statusCode,
  });
});

// 4. Expose the /metrics endpoint
app.get('/metrics', async (req, res) => {
  try {
    res.set('Content-Type', register.contentType);
    res.end(await register.metrics());
  } catch (ex) {
    res.status(500).end(ex);
  }
});

app.listen(PORT, () => {
  console.log(`Service-A listening on port ${PORT}`);
});