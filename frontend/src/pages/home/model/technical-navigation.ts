import { generatePath, matchPath } from 'react-router'
import { routePaths } from '@/shared/config'

export const technicalNavigation = [
  {
    pattern: routePaths.findings,
    path: routePaths.findings,
    label: 'Находки',
    action: 'findings.read',
  },
  {
    pattern: routePaths.componentOverview,
    path: generatePath(routePaths.componentOverview),
    label: 'Технический обзор компонента',
    action: 'findings.read',
  },
  {
    pattern: routePaths.serviceDocuments,
    path: generatePath(routePaths.serviceDocuments),
    label: 'Документация сервиса',
    action: 'docs.read',
  },
  {
    pattern: routePaths.checkProfile,
    path: generatePath(routePaths.checkProfile),
    label: 'Матрица проверок',
    action: 'check_profiles.manage',
  },
  {
    pattern: routePaths.ciConfiguration,
    path: generatePath(routePaths.ciConfiguration),
    label: 'Подключение шага CI',
    action: 'check_profiles.manage',
  },
]

const technicalDetails = [
  { pattern: routePaths.findingDetail, action: 'findings.read' },
  { pattern: routePaths.technicalComparisonSetup, action: 'findings.read' },
  { pattern: routePaths.technicalComparison, action: 'findings.read' },
  { pattern: routePaths.exceptions, action: 'findings.read' },
  { pattern: routePaths.exceptionCreate, action: 'exceptions.manage' },
  { pattern: routePaths.exceptionExtend, action: 'exceptions.manage' },
  { pattern: routePaths.pipeline, action: 'ci.read' },
  { pattern: routePaths.pipelineJob, action: 'ci.read' },
  { pattern: routePaths.baselines, action: 'ci.read' },
  { pattern: routePaths.mergeRequestSummary, action: 'mr.read' },
  { pattern: routePaths.documentationRoots, action: 'docs.read' },
  { pattern: routePaths.documentationFinding, action: 'findings.read' },
]

export function technicalRouteAction(pathname: string) {
  return [...technicalNavigation, ...technicalDetails].find(({ pattern }) =>
    matchPath(pattern, pathname),
  )?.action
}
