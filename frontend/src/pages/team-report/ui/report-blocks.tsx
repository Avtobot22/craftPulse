import type { ApiClient } from '@/shared/api'
import type { ReportBlockProps } from '../model/metrics'
import { CompletenessBlock } from './completeness-block'
import { CycleTimeBlock } from './cycle-time-block'
import { FlowBalanceBlock } from './flow-balance-block'
import { ReopeningsBlock, TargetComplianceBlock } from './outcome-metrics-block'
import { ThroughputBlock } from './throughput-block'
import { WorkAgeBlock } from './work-age-block'

export function ReportBlocks({
  block,
  api,
  canReadVersions,
}: {
  block: ReportBlockProps
  api: ApiClient
  canReadVersions: boolean
}) {
  if (block.report.metrics.length === 0) return null
  return (
    <div className="cp-team-report__blocks" key={block.report.revision.id}>
      <ThroughputBlock block={block} api={api} />
      <CycleTimeBlock block={block} api={api} />
      <FlowBalanceBlock block={block} />
      <WorkAgeBlock block={block} api={api} />
      <TargetComplianceBlock
        block={block}
        api={api}
        canReadVersions={canReadVersions}
      />
      <ReopeningsBlock block={block} />
      <CompletenessBlock block={block} />
    </div>
  )
}
