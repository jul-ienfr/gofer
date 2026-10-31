import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = (__ENV.BASE_URL || 'http://127.0.0.1:8090').replace(/\/+$/, '');
const COOKIE = (__ENV.GOFER_COOKIE || '').trim();
const APP_PATHS = (__ENV.APP_PATHS || '/,/calendar,/contacts')
  .split(',')
  .map((path) => path.trim())
  .filter(Boolean);
const DURATION = __ENV.DURATION || '5m';
const HEALTH_RPS = Number(__ENV.HEALTH_RPS || 10);
const APP_ENABLED = COOKIE !== '' && __ENV.SKIP_APP !== '1';

function parseStages(value) {
  const stages = value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => {
      const [duration, target] = entry.split(':').map((part) => part.trim());
      if (!duration || !target || Number.isNaN(Number(target))) {
        throw new Error(`STAGES entry must look like 30s:20, got ${entry}`);
      }
      return { duration, target: Number(target) };
    });
  if (stages.length === 0) {
    throw new Error('STAGES must contain at least one duration:target entry');
  }
  return stages;
}

const appStages = __ENV.STAGES
  ? parseStages(__ENV.STAGES)
  : [
      { duration: '30s', target: 5 },
      { duration: '2m', target: 10 },
      { duration: '2m', target: 20 },
      { duration: '1m', target: 20 },
      { duration: '30s', target: 0 },
    ];

const appScenario = APP_ENABLED
  ? {
      app: {
        executor: 'ramping-vus',
        startVUs: 1,
        stages: appStages,
        gracefulStop: '30s',
        exec: 'app',
      },
    }
  : {};

export const options = {
  scenarios: {
    health: {
      executor: 'constant-arrival-rate',
      rate: HEALTH_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: 10,
      maxVUs: 100,
      exec: 'health',
    },
    ...appScenario,
  },
  thresholds: {
    checks: ['rate>0.99'],
    'http_req_failed': ['rate<0.01'],
    'http_req_failed{scenario:health}': ['rate<0.001'],
    'http_req_duration{scenario:health}': ['p(95)<250', 'p(99)<500'],
    'http_req_duration{scenario:app}': ['p(95)<1500', 'p(99)<3000'],
  },
};

export function health() {
  const response = http.get(`${BASE_URL}/healthz`, { tags: { kind: 'health' } });
  check(response, {
    'health status is 200': (result) => result.status === 200,
    'health body is not empty': (result) => (result.body || '').length > 0,
  });
  sleep(0.1);
}

export function app() {
  const path = APP_PATHS[Math.floor(Math.random() * APP_PATHS.length)];
  const response = http.get(`${BASE_URL}${path}`, {
    headers: COOKIE ? { Cookie: COOKIE } : {},
    tags: { kind: 'app', path },
  });
  check(response, {
    'app page is 200': (result) => result.status === 200,
    'app page is not a login redirect': (result) =>
      !String(result.url || '').includes('/login'),
    'app page has content': (result) => (result.body || '').length > 1000,
  });
  sleep(1 + Math.random() * 2);
}
