import http from 'k6/http';
import { check, sleep } from 'k6';

// No custom metric is needed.

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '3m', target: 50 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    // We only need to check the built-in duration metric
    'http_req_duration': ['p(99) < 750'],
    'http_req_failed': ['rate < 0.001'],
  },
};

export default function () {
  const res = http.get('http://localhost:8080/');

  check(res, {
    'status was 200': (r) => r.status === 200,
  });

  // We don't add to the custom metric anymore.
  sleep(1);
}