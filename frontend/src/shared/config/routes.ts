export const routePaths = Object.freeze({
  home: '/',
  login: '/login',
  sessionExpired: '/session-expired',
  unavailable: '/access-denied',
  installation: '/installation',
  welcome: '/welcome',
  mergeRequestSummary: '/merge-requests/:mr/summary',
  organization: '/organization/structure',
  users: '/users',
  workflowSettings: '/settings/process',
  targetSettings: '/settings/targets',
  teamReport: '/reports/team',
  metricPassport: '/reports/revisions/:revisionId/metrics/:metricId',
  metricCohort:
    '/reports/revisions/:revisionId/metrics/:metricId/points/:pointStart',
  savedView: '/reports/saved-views/:savedViewId',
})
