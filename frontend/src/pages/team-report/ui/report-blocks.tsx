import type { ApiClient } from '@/shared/api'
import type { TeamReportScope } from '@/shared/config'
import type { ReportBlockProps } from '../model/metrics'
import { CompletenessBlock } from './completeness-block'
import { CycleTimeBlock } from './cycle-time-block'
import { FlowBalanceBlock } from './flow-balance-block'
import { ReopeningsBlock, TargetComplianceBlock } from './outcome-metrics-block'
import { ThroughputBlock } from './throughput-block'
import { CrossMetricsBlock } from './cross-metrics-block'
import { TeamMRQueue } from './team-mr-queue'
import { WorkAgeBlock } from './work-age-block'

export function ReportBlocks({
  block,
  api,
  canReadVersions,
  canReadTasks,
  queueProjects,
}: {
  block: ReportBlockProps
  api: ApiClient
  canReadVersions: boolean
  canReadTasks?: boolean
  queueProjects?: TeamReportScope['projects']
}) {
  return (
    <div className="cp-team-report__blocks" key={block.report.revision.id}>
      {canReadTasks !== undefined && (
        <CrossMetricsBlock
          block={block}
          api={api}
          canReadTasks={canReadTasks}
        />
      )}
      {queueProjects && (
        <TeamMRQueue api={api} report={block.report} projects={queueProjects} />
      )}
      {block.report.metrics.length > 0 && (
        <>
          <ThroughputBlock block={block} api={api} />
          <CycleTimeBlock block={block} api={api} />
          <FlowBalanceBlock block={block} />
          <WorkAgeBlock
            block={block}
            api={api}
            canReadTasks={canReadTasks ?? false}
          />
          <TargetComplianceBlock
            block={block}
            api={api}
            canReadVersions={canReadVersions}
          />
          <ReopeningsBlock block={block} />
          <CompletenessBlock block={block} />
        </>
      )}
    </div>
  )
}
