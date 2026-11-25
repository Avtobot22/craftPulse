// A separate D-load series for the first page of listTasks after five warm-up requests per session.
import http from 'k6/http';
import exec from 'k6/execution';

const manifest = JSON.parse(open(__ENV.MANIFEST));
const warmup = 5;
const measured = 20;

export const options = {
  scenarios: {
    d_load_tasks: {
      executor: 'per-vu-iterations',
      vus: manifest.sessions.length,
      iterations: warmup + measured,
      maxDuration: '10m',
    },
  },
  thresholds: {
    'http_req_duration{phase:measure,op:listTasks}': ['p(95)<=2000', 'max<=5000'],
    'http_req_failed{phase:measure,op:listTasks}': ['rate==0'],
    'http_reqs{phase:measure,op:listTasks}': ['count==100'],
  },
  discardResponseBodies: true,
  summaryTrendStats: ['min', 'med', 'p(95)', 'p(99)', 'max', 'count'],
};

export default function () {
  const vu = exec.vu.idInTest - 1;
  const cycle = exec.vu.iterationInScenario;
  const phase = cycle < warmup ? 'warmup' : 'measure';
  const filter = manifest.filters[(vu * 7 + cycle) % manifest.filters.length];
  const query = Object.entries(filter)
    .map(([name, value]) => `${encodeURIComponent(name)}=${encodeURIComponent(value)}`)
    .join('&');
  http.get(`${manifest.base_url}/api/v1/tasks?${query}`, {
    headers: { Cookie: manifest.sessions[vu] },
    tags: { phase, op: 'listTasks' },
    responseCallback: http.expectedStatuses(200),
  });
}

export function handleSummary(data) {
  return { [__ENV.SUMMARY]: JSON.stringify(data, null, 2) };
}
