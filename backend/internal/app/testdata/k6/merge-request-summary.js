// MANIFEST is a JSON file with base_url, five session cookies in sessions,
// and at least five MR UUIDs on the MVP dataset in merge_request_ids.
import http from 'k6/http';
import exec from 'k6/execution';

const manifest = JSON.parse(open(__ENV.MANIFEST));
const warmup = 5;
const measured = 20;
const sessions = 5;

if (manifest.sessions.length !== sessions || new Set(manifest.merge_request_ids).size < sessions) {
  throw new Error('Provide five sessions and at least five distinct merge requests');
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
    'http_req_duration{phase:measure,op:getMergeRequestSummary}': ['p(95)<=2000', 'max<=5000'],
    'http_reqs{phase:measure,op:getMergeRequestSummary}': ['count==100'],
    'http_req_failed{phase:measure,op:getMergeRequestSummary}': ['rate==0'],
  },
  discardResponseBodies: true,
  summaryTrendStats: ['min', 'med', 'p(95)', 'p(99)', 'max', 'count'],
};

export default function () {
  const vu = exec.vu.idInTest - 1;
  const cycle = exec.vu.iterationInScenario;
  const phase = cycle < warmup ? 'warmup' : 'measure';
  const mr = manifest.merge_request_ids[(vu + cycle) % manifest.merge_request_ids.length];
  http.get(`${manifest.base_url}/api/v1/merge-requests/${encodeURIComponent(mr)}/summary`, {
    headers: { Cookie: manifest.sessions[vu] },
    tags: { phase, op: 'getMergeRequestSummary' },
    responseCallback: http.expectedStatuses(200),
  });
}
