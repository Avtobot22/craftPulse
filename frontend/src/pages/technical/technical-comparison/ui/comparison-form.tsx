import type { SubmitEvent } from 'react'
import { generatePath, useLocation, useNavigate } from 'react-router'
import { routePaths, technicalLabels } from '@/shared/config'

export function ComparisonForm({
  componentId,
  params,
}: {
  componentId: string
  params: URLSearchParams
}) {
  const navigate = useNavigate()
  const state: unknown = useLocation().state

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const next = new URLSearchParams(params)
    next.delete('finding_id')
    const selectedComponent = data.get('component_id')
    if (typeof selectedComponent !== 'string') return
    for (const field of ['base_sha', 'head_sha', 'analysis_kind']) {
      const value = data.get(field)
      if (typeof value === 'string') next.set(field, value.trim())
    }
    void navigate(
      {
        pathname: generatePath(routePaths.technicalComparison, {
          componentId: selectedComponent.trim().toLowerCase(),
        }),
        search: next.toString(),
      },
      { replace: true, state },
    )
  }

  return (
    <form className="cp-comparison-form" onSubmit={submit}>
      <label>
        ID компонента
        <input
          name="component_id"
          required
          pattern="[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}"
          defaultValue={componentId}
        />
      </label>
      <label>
        SHA базы
        <input
          name="base_sha"
          required
          pattern="[0-9a-f]{40}|[0-9a-f]{64}"
          defaultValue={params.get('base_sha') ?? ''}
        />
      </label>
      <label>
        SHA head
        <input
          name="head_sha"
          required
          pattern="[0-9a-f]{40}|[0-9a-f]{64}"
          defaultValue={params.get('head_sha') ?? ''}
        />
      </label>
      <label>
        Вид анализа
        <select
          name="analysis_kind"
          required
          defaultValue={params.get('analysis_kind') ?? ''}
        >
          <option value="">Выберите вид анализа</option>
          {Object.entries(technicalLabels.analysisKind).map(([code, label]) => (
            <option key={code} value={code}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <button type="submit">Сравнить</button>
    </form>
  )
}
