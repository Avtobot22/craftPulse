// MANIFEST contains base_url, five session cookies in sessions, and at least
// five distinct service UUIDs with assessed documentation in service_ids.
import http from 'k6/http';
import exec from 'k6/execution';

const manifest = JSON.parse(open(__ENV.MANIFEST));
const warmup = 5;
const measured = 20;
const sessions = 5;

if (manifest.sessions.length !== sessions || new Set(manifest.service_ids).size < sessions) {
  throw new Error('Provide five sessions and at least five distinct services');
}

export const options = {
  scenarios: {
    d_load: {
      executor: 'per-vu-iterations',
      vus: sessions,
      iterations: warmup + measured,
      maxDuration: '10m',
    },
  },
  thresholds: {
    'http_req_duration{phase:measure,op:listServiceDocuments}': ['p(95)<=2000', 'max<=5000'],
    'http_reqs{phase:measure,op:listServiceDocuments}': ['count==100'],
    'http_req_failed{phase:measure,op:listServiceDocuments}': ['rate==0'],
  },
  discardResponseBodies: true,
  summaryTrendStats: ['min', 'med', 'p(95)', 'p(99)', 'max', 'count'],
};

export default function () {
  const vu = exec.vu.idInTest - 1;
  const cycle = exec.vu.iterationInScenario;
  const phase = cycle < warmup ? 'warmup' : 'measure';
  const service = manifest.service_ids[(vu + cycle) % manifest.service_ids.length];
  http.get(`${manifest.base_url}/api/v1/services/${encodeURIComponent(service)}/documents`, {
    headers: { Cookie: manifest.sessions[vu] },
    tags: { phase, op: 'listServiceDocuments' },
    responseCallback: http.expectedStatuses(200),
  });
}
