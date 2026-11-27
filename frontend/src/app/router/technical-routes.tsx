import type { RouteObject } from 'react-router'
import type { ApiClient } from '@/shared/api'
import { ComponentOverviewPage } from '@/pages/technical/component-overview'
import { FindingsPage } from '@/pages/technical/findings'
import { FindingDetailPage } from '@/pages/technical/finding-detail'
import { ExceptionFormPage } from '@/pages/technical/exception-form'
import { ExceptionsPage } from '@/pages/technical/exceptions'
import { CheckProfilePage } from '@/pages/technical/check-profile'
import { CiConfigurationPage } from '@/pages/technical/ci-configuration'
import { BaselinesPage } from '@/pages/technical/baselines'
import { TechnicalComparisonPage } from '@/pages/technical/technical-comparison'
import { DocumentationRootsPage } from '@/pages/technical/documentation-roots'
import { ServiceDocumentsPage } from '@/pages/technical/service-documents'
import { DocumentationFindingPage } from '@/pages/technical/documentation-finding'
import { routePaths } from '@/shared/config'

// UI-44: componentId. UI-45: component_id, project_id, service_id,
// finding_kind, state, severity, sort, limit and cursor in the search string.
// UI-46: findingId. UI-47: findingId for creation or exceptionId for extension.
// UI-48: optional exceptionId. Search filters: state, component_id, finding_id,
// sort, limit and cursor.
// UI-49/UI-52: projectId. UI-50: projectId and ci_system in the search string.
// UI-53 (routes.tsx): mr with optional revision_no and head_sha search params.
// UI-55/UI-56 (routes.tsx): pipelineId and, for UI-56, URL-encoded jobKey.
// UI-57: componentId and base_sha, head_sha, analysis_kind in the search string.
// UI-64/UI-65: serviceId. UI-66: findingId.
// A missing optional path ID opens the section before an object is selected.
export const technicalRoutes = (api: ApiClient) =>
  [
    { path: routePaths.componentOverview, element: <ComponentOverviewPage /> },
    { path: routePaths.findings, element: <FindingsPage /> },
    {
      path: routePaths.findingDetail,
      element: <FindingDetailPage api={api} />,
    },
    { path: routePaths.exceptionCreate, element: <ExceptionFormPage /> },
    { path: routePaths.exceptionExtend, element: <ExceptionFormPage /> },
    { path: routePaths.exceptions, element: <ExceptionsPage /> },
    { path: routePaths.checkProfile, element: <CheckProfilePage /> },
    { path: routePaths.ciConfiguration, element: <CiConfigurationPage /> },
    { path: routePaths.baselines, element: <BaselinesPage /> },
    {
      path: routePaths.technicalComparison,
      element: <TechnicalComparisonPage api={api} />,
    },
    {
      path: routePaths.technicalComparisonSetup,
      element: <TechnicalComparisonPage api={api} />,
    },
    {
      path: routePaths.documentationRoots,
      element: <DocumentationRootsPage />,
    },
    { path: routePaths.serviceDocuments, element: <ServiceDocumentsPage /> },
    {
      path: routePaths.documentationFinding,
      element: <DocumentationFindingPage />,
    },
  ] satisfies RouteObject[]
