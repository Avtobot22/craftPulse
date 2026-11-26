import http from 'k6/http';
import exec from 'k6/execution';

const manifest = JSON.parse(open(__ENV.MANIFEST));
const warmup = 5;
const measured = 20;
const vus = 5;
const thresholds = {};

for (const op of Object.keys(manifest.paths)) {
  thresholds[`http_req_duration{phase:measure,op:${op}}`] = ['p(95)<=2000', 'max<=5000'];
  thresholds[`http_req_failed{phase:measure,op:${op}}`] = ['rate==0'];
  thresholds[`http_reqs{phase:measure,op:${op}}`] = [`count==${vus * measured}`];
}

export const options = {
  scenarios: {
    findings: { executor: 'per-vu-iterations', vus, iterations: warmup + measured, maxDuration: '10m' },
  },
  thresholds,
  discardResponseBodies: true,
  summaryTrendStats: ['min', 'med', 'p(95)', 'p(99)', 'max', 'count'],
};

http.setResponseCallback(http.expectedStatuses(200));

export default function () {
  const vu = exec.vu.idInTest - 1;
  const cycle = exec.vu.iterationInScenario;
  const phase = cycle < warmup ? 'warmup' : 'measure';
  for (const [op, paths] of Object.entries(manifest.paths)) {
    const path = paths[(vu + cycle) % paths.length];
    http.get(manifest.base_url + path, { headers: { Cookie: manifest.sessions[vu] }, tags: { phase, op } });
  }
}

export function handleSummary(data) {
  return { [__ENV.SUMMARY]: JSON.stringify(data, null, 2) };
}
