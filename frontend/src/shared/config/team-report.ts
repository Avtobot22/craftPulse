export interface ReportPointFocus {
  locationKey: string
  targetId: string
  seriesId: string
  pointStart: string
  revisionId: string
  chartSeries?: string
}

export interface TeamReportScope {
  team?: { id: string; name: string }
  project?: { id: string; name: string }
  projects: readonly { id: string; name: string }[]
  canManageProcess: boolean
  focus: ReportPointFocus | undefined
  rememberPoint: (point: ReportPointFocus | undefined) => void
}

export const teamReportReturnState = { fromTeamReport: true }
