import type { RouteObject } from 'react-router'
import { ConnectionFormPage } from '@/pages/administration/connection-form'
import { ConnectionsPage } from '@/pages/administration/connections'
import { ImportPackagePage } from '@/pages/administration/import-package'
import { IngestConflictsPage } from '@/pages/administration/ingest-conflicts'
import { ReceiptPage } from '@/pages/administration/receipt'
import { ReceiptJournalPage } from '@/pages/administration/receipt-journal'
import { SourceAccountsPage } from '@/pages/administration/source-accounts'
import { SyncStatusPage } from '@/pages/administration/sync-status'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'

// UI-10/UI-11: connectionId. UI-13: receiptId.
// Search parameters and link builders for screen transitions: shared/config/admin-routes.
export const adminRoutes = (api: ApiClient) =>
  [
    { path: routePaths.connections, element: <ConnectionsPage api={api} /> },
    {
      path: routePaths.connectionNew,
      element: <ConnectionFormPage api={api} />,
    },
    {
      path: routePaths.connectionDetail,
      element: <ConnectionFormPage api={api} />,
    },
    { path: routePaths.syncStatus, element: <SyncStatusPage api={api} /> },
    {
      path: routePaths.ingestConflicts,
      element: <IngestConflictsPage api={api} />,
    },
    {
      path: routePaths.importPackage,
      element: <ImportPackagePage api={api} />,
    },
    { path: routePaths.receiptJournal, element: <ReceiptJournalPage /> },
    { path: routePaths.receipt, element: <ReceiptPage api={api} /> },
    { path: routePaths.sourceAccounts, element: <SourceAccountsPage /> },
  ] satisfies RouteObject[]
