import { generatePath } from 'react-router'
import type { components, OpenCodes } from '@/shared/api'
import { routePaths, technicalLabels } from '@/shared/config'

export type FindingDetail = OpenCodes<components['schemas']['FindingDetail']>

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu
const shaPattern = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u
const analysisByFinding = {
  code: 'code',
  security: 'sast',
  secret: 'secrets',
  dependency: 'sca',
  iac: 'iac',
  docs: 'docs',
  openapi: 'openapi',
} satisfies Record<
  components['schemas']['FindingKind'],
  components['schemas']['AnalysisKind']
>

export function readFindingContext(search: URLSearchParams) {
  const mr = search.get('mr_id') ?? ''
  const revision = search.get('revision_no') ?? ''
  const head = search.get('head_sha') ?? ''
  const base = search.get('base_sha') ?? ''
  const kind = search.get('analysis_kind')
  const scope = new URLSearchParams()
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = search.get(key)
    if (value) scope.set(key, value)
  }

  const mrId = uuid.test(mr) ? mr.toLowerCase() : undefined
  const headSha = shaPattern.test(head) ? head : undefined
  const baseSha = shaPattern.test(base) ? base : undefined
  const summarySearch = new URLSearchParams(scope)
  if (/^[1-9][0-9]{0,9}$/u.test(revision) && Number(revision) <= 2147483647)
    summarySearch.set('revision_no', revision)
  if (headSha) summarySearch.set('head_sha', headSha)

  return {
    summary: mrId
      ? {
          pathname: generatePath(routePaths.mergeRequestSummary, { mr: mrId }),
          search: summarySearch.toString(),
        }
      : undefined,
    scope,
    headSha,
    baseSha,
    kind:
      kind && Object.hasOwn(technicalLabels.analysisKind, kind)
        ? kind
        : undefined,
  }
}

export function findingComparisonLink(
  finding: FindingDetail,
  context: ReturnType<typeof readFindingContext>,
) {
  const { component_id, report, finding_kind } = finding
  const search = new URLSearchParams(context.scope)
  const sha = report.sha && shaPattern.test(report.sha) ? report.sha : null
  if (sha) {
    search.set('head_sha', sha)
    if (sha === context.headSha && context.baseSha)
      search.set('base_sha', context.baseSha)
  }
  const kind =
    context.kind ??
    Object.entries(analysisByFinding).find(
      ([findingKind]) => findingKind === finding_kind,
    )?.[1]
  if (kind) search.set('analysis_kind', kind)
  if (!component_id) search.set('finding_id', finding.id)
  return {
    pathname: component_id
      ? generatePath(routePaths.technicalComparison, {
          componentId: encodeURIComponent(component_id),
        })
      : routePaths.technicalComparisonSetup,
    search: search.toString(),
  }
}

export function gitLabLocationUrl(source: string | undefined) {
  const url = source ? URL.parse(source) : null
  if (
    !url ||
    (url.protocol !== 'http:' && url.protocol !== 'https:') ||
    url.username ||
    url.password
  )
    return undefined

  return url.href
}
