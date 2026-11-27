import { useQuery } from '@tanstack/react-query'
import { generatePath, Link, useSearchParams } from 'react-router'
import { getCiConfiguration } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { CopyButton } from './copy-button'
import { UploadTokens } from './upload-tokens'
import './ci-configuration.css'

export function CiConfigurationPage({
  api,
  projectId,
  canManageTokens,
  canReadBaselines,
}: {
  api: ApiClient
  projectId: string
  canManageTokens: boolean
  canReadBaselines: boolean
}) {
  const [params, setParams] = useSearchParams()
  const system = params.get('ci_system') ?? 'gitlab_ci'

  return (
    <div className="cp-ci-setup">
      <nav aria-label="Проверки проекта">
        <Link to={generatePath(routePaths.checkProfile, { projectId })}>
          К матрице проверок
        </Link>
        {canReadBaselines && (
          <Link to={generatePath(routePaths.baselines, { projectId })}>
            Базовые линии
          </Link>
        )}
      </nav>
      <label>
        Система CI
        <select
          value={system === 'gitlab_ci' ? system : ''}
          onChange={() => {
            setParams((previous) => {
              const next = new URLSearchParams(previous)
              next.set('ci_system', 'gitlab_ci')
              return next
            })
          }}
        >
          {system !== 'gitlab_ci' && (
            <option value="" disabled>
              Выберите систему CI
            </option>
          )}
          <option value="gitlab_ci">GitLab CI</option>
        </select>
      </label>
      {system === 'gitlab_ci' ? (
        <GitLabConfiguration
          api={api}
          projectId={projectId}
          canReadBaselines={canReadBaselines}
        />
      ) : (
        <p role="status">
          Для выбранной системы CI инструкция пока недоступна.
        </p>
      )}
      {canManageTokens ? (
        <UploadTokens api={api} projectId={projectId} />
      ) : (
        <section aria-label="Токены загрузки">
          <h2>Токены загрузки</h2>
          <p>
            Для выпуска и отзыва токенов обратитесь к администратору проекта.
          </p>
        </section>
      )}
    </div>
  )
}

function GitLabConfiguration({
  api,
  projectId,
  canReadBaselines,
}: {
  api: ApiClient
  projectId: string
  canReadBaselines: boolean
}) {
  const query = useQuery(
    getCiConfiguration(api, projectId, { ci_system: 'gitlab_ci' }),
  )
  if (query.isPending) return <LoadingState />
  if (query.isError)
    return (
      <ErrorState
        reason="Не удалось загрузить конфигурацию CI"
        onRetry={() => void query.refetch()}
        retrying={query.isFetching}
      />
    )

  const configuration = query.data
  if (configuration.ci_system !== 'gitlab_ci')
    return (
      <p role="status">Для выбранной системы CI инструкция пока недоступна.</p>
    )

  return (
    <section aria-label="Инструкция GitLab CI">
      <h2>Фрагмент .gitlab-ci.yml</h2>
      <p>
        Версия матрицы {configuration.profile_version_no} · Режим предупреждений
      </p>
      <pre tabIndex={0} aria-label="Конфигурация GitLab CI">
        <code>{configuration.snippet}</code>
      </pre>
      <CopyButton
        key={configuration.snippet}
        value={configuration.snippet}
        label="Скопировать конфигурацию"
      />
      <h3>Переменные CI</h3>
      <ul>
        {configuration.variables.map(({ name, masked }) => (
          <li key={name}>
            <code>{name}</code>
            {masked && ' · Маскированная'}
          </li>
        ))}
      </ul>
      <ol>
        <li>
          В настройках проекта GitLab откройте Settings → CI/CD → Variables.
          Добавьте переменные из списка. Для CRAFTPULSE_UPLOAD_TOKEN включите
          маскирование и проверьте доступность переменной в нужных MR-pipelines.
        </li>
        <li>
          Вставьте фрагмент в .gitlab-ci.yml обычным MR, сохранив действующие
          проверки.
        </li>
        <li>
          После слияния дождитесь полного pipeline основной ветки и{' '}
          {canReadBaselines ? (
            <Link to={generatePath(routePaths.baselines, { projectId })}>
              откройте базовые линии проекта
            </Link>
          ) : (
            'попросите коллегу с доступом к CI проверить базовые линии проекта'
          )}
          .
        </li>
      </ol>
    </section>
  )
}
