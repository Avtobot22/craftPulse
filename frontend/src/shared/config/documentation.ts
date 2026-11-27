import { generatePath } from 'react-router'
import { routePaths } from './routes'
import { technicalScope } from './technical-context'

export function documentationCorpus(
  search: URLSearchParams,
  serviceId?: string,
) {
  return {
    pathname: generatePath(routePaths.serviceDocuments, { serviceId }),
    search: technicalScope(search).toString(),
  }
}

export function documentationRoots(search: URLSearchParams, serviceId: string) {
  return {
    pathname: generatePath(routePaths.documentationRoots, { serviceId }),
    search: technicalScope(search).toString(),
  }
}

export function documentationFinding(
  search: URLSearchParams,
  serviceId: string,
  findingId: string,
) {
  const context = technicalScope(search)
  context.set('service_id', serviceId)
  return {
    pathname: generatePath(routePaths.documentationFinding, { findingId }),
    search: context.toString(),
  }
}
