import { routePaths } from '@/shared/config'

const connections = {
  path: routePaths.connections,
  action: 'connections.manage',
  label: 'Подключения',
}

export const adminNavigation = [
  connections,
  {
    path: routePaths.ingestConflicts,
    action: 'sync.read',
    label: 'Конфликты приёма',
  },
  { path: routePaths.importPackage, action: 'imports.create', label: 'Импорт' },
  {
    path: routePaths.receiptJournal,
    action: 'sync.read',
    label: 'Журнал приёма',
  },
  {
    path: routePaths.sourceAccounts,
    action: 'accounts.manage',
    label: 'Учётные записи',
  },
]

export const adminDetailRoutes = [
  {
    ...connections,
    path: routePaths.connectionNew,
    label: 'Новое подключение',
  },
  {
    ...connections,
    path: routePaths.connectionDetail,
    label: 'Настройки подключения',
  },
  {
    path: routePaths.syncStatus,
    action: 'sync.read',
    label: 'Состояние сбора',
  },
  {
    path: routePaths.receipt,
    action: 'receipts.read',
    label: 'Квитанция приёма',
  },
]
