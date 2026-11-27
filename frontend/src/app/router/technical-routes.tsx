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
import { CheckProjectRoute } from './check-project-route'

// UI-44: componentId. UI-45: component_id, project_id, service_id,
// finding_kind, state, severity, sort, limit and cursor in the search string.
// UI-46: findingId. UI-47: findingId for creation or exceptionId for extension.
// UI-48: optional exceptionId. Search filters: state, component_id, finding_id,
// sort, limit and cursor.
// UI-49/UI-52: optional projectId. Baseline evidence: projectId and reportId.
// UI-50: projectId and ci_system in the search string.
// UI-53 (routes.tsx): mr with optional revision_no and head_sha search params.
// UI-55/UI-56 (routes.tsx): pipelineId and, for UI-56, URL-encoded jobKey.
// UI-57: componentId and base_sha, head_sha, analysis_kind in the search string.
// UI-64/UI-65: serviceId. UI-66: findingId.
// A missing optional path ID opens the section before an object is selected.
export const technicalRoutes = (api: ApiClient) =>
  [
    {
      path: routePaths.componentOverview,
      element: <ComponentOverviewPage api={api} />,
    },
    { path: routePaths.findings, element: <FindingsPage api={api} /> },
    {
      path: routePaths.findingDetail,
      element: <FindingDetailPage api={api} />,
    },
    {
      path: routePaths.exceptionCreate,
      element: <ExceptionFormPage api={api} />,
    },
    {
      path: routePaths.exceptionExtend,
      element: <ExceptionFormPage api={api} />,
    },
    { path: routePaths.exceptions, element: <ExceptionsPage api={api} /> },
    {
      path: routePaths.checkProfile,
      element: (
        <CheckProjectRoute
          title="Матрица проверок"
          path={routePaths.checkProfile}
        >
          {(projectId, actions) => (
            <CheckProfilePage
              key={projectId}
              api={api}
              projectId={projectId}
              canReadBaselines={actions.includes('ci.read')}
            />
          )}
        </CheckProjectRoute>
      ),
    },
    {
      path: routePaths.ciConfiguration,
      element: (
        <CheckProjectRoute
          title="Подключение шага CI"
          path={routePaths.ciConfiguration}
        >
          {(projectId, actions) => (
            <CiConfigurationPage
              key={projectId}
              api={api}
              projectId={projectId}
              canManageTokens={actions.includes('upload_tokens.manage')}
              canReadBaselines={actions.includes('ci.read')}
            />
          )}
        </CheckProjectRoute>
      ),
    },
    { path: routePaths.baselines, element: <BaselinesPage api={api} /> },
    { path: routePaths.baselineReport, element: <BaselinesPage api={api} /> },
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
      element: <DocumentationRootsPage api={api} />,
    },
    {
      path: routePaths.serviceDocuments,
      element: <ServiceDocumentsPage api={api} />,
    },
    {
      path: routePaths.documentationFinding,
      element: <DocumentationFindingPage api={api} />,
    },
  ] satisfies RouteObject[]
