// D-load of TZ 8.2 for the report of a team (MVP-04-T08): five sessions of the first organization, each without
// pauses runs 5 warm-up cycles and 20 measured ones. A cycle opens the weekly report with a filter, the monthly
// chart, the cohort of a non-zero point and a task card. The report-only run opens a revision passport before
// the cohort. The manifest fixes sessions, filters, points and tasks. Thresholds hold only for measured cycles:
// p95 <= 2 s, max <= 5 s and no errors for each operation.
import http from 'k6/http';
import exec from 'k6/execution';

const manifest = JSON.parse(open(__ENV.MANIFEST));
const warmup = 5;
const measured = 20;
const operations = ['getTeamReport', 'listRevisionCohort', manifest.task_paths ? 'getTask' : 'getRevision'];

const thresholds = {};
for (const op of operations) {
  thresholds[`http_req_duration{phase:measure,op:${op}}`] = ['p(95)<=2000', 'max<=5000'];
  thresholds[`http_req_failed{phase:measure,op:${op}}`] = ['rate==0'];
  thresholds[`http_reqs{phase:measure,op:${op}}`] = [op === 'getTeamReport' ? 'count==200' : 'count==100'];
}

export const options = {
  scenarios: {
    d_load: {
      executor: 'per-vu-iterations',
      vus: manifest.sessions.length,
      iterations: warmup + measured,
      maxDuration: '10m',
    },
  },
  thresholds,
  discardResponseBodies: true,
  summaryTrendStats: ['min', 'med', 'p(95)', 'p(99)', 'max', 'count'],
};

function query(values) {
  return Object.entries(values)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&');
}

export default function () {
  const vu = exec.vu.idInTest - 1;
  const cycle = exec.vu.iterationInScenario;
  const phase = cycle < warmup ? 'warmup' : 'measure';
  const params = (op) => ({ headers: { Cookie: manifest.sessions[vu] }, tags: { phase, op } });
  const pick = (list) => list[(vu * 7 + cycle) % list.length];
  const base = `${manifest.base_url}/api/v1`;
  const period = { from: manifest.from, to: manifest.to };

  const filter = pick(manifest.filters);
  http.get(`${base}/teams/${manifest.team_id}/report?${query({ ...period, ...filter })}`, params('getTeamReport'));
  http.get(`${base}/teams/${manifest.team_id}/report?${query({ ...period, step: 'month' })}`, params('getTeamReport'));
  if (!manifest.task_paths) {
    http.get(`${base}/revisions/${manifest.revision_id}`, params('getRevision'));
  }
  const point = pick(manifest.points);
  http.get(`${base}/revisions/${manifest.revision_id}/cohort?${query(point)}`, params('listRevisionCohort'));
  if (manifest.task_paths) {
    http.get(`${manifest.base_url}${pick(manifest.task_paths)}`, params('getTask'));
  }
}

export function handleSummary(data) {
  return { [__ENV.SUMMARY]: JSON.stringify(data, null, 2) };
}
