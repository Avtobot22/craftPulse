import { CircleCheck, Clock } from '@gravity-ui/icons'
import { Button, Icon } from '@gravity-ui/uikit'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { DialogFrame, ErrorState, UnavailableRedirect } from '@/shared/ui'
import { usersKey } from '../api/users-api'
import type { User } from '../api/users-api'
import { userName } from './user-list'

const actions = {
  block: {
    label: 'Заблокировать',
    description:
      'Все сессии пользователя завершатся, а его ключи API перестанут действовать.',
    result: 'Пользователь заблокирован.',
  },
  unblock: {
    label: 'Разблокировать',
    description:
      'Пользователь сможет войти снова. Завершённые сессии и отключённые ключи API не восстановятся.',
    result: 'Пользователь разблокирован.',
  },
  sessions: {
    label: 'Завершить все сессии',
    description:
      'Все сессии пользователя завершатся. Для продолжения работы потребуется заново войти.',
    result: 'Все сессии пользователя завершены.',
  },
}
type Action = keyof typeof actions

export function UserAccess({
  api,
  user,
  statusCurrent,
}: {
  api: ApiClient
  user: User
  statusCurrent: boolean
}) {
  const client = useQueryClient()
  const [pending, setPending] = useState<Action>()
  const [notice, setNotice] = useState('')
  const statusButton = useRef<HTMLButtonElement>(null)
  const sessionsButton = useRef<HTMLButtonElement>(null)
  const mutation = useMutation({
    mutationFn: (action: Action) =>
      api.changeUserAccess(user.id, action, AbortSignal.timeout(30_000)),
    onSuccess: async (_, action) => {
      await client.invalidateQueries({ queryKey: usersKey })
      setPending(undefined)
      setNotice(actions[action].result)
    },
  })
  const statusAction =
    user.status === 'active'
      ? 'block'
      : user.status === 'blocked'
        ? 'unblock'
        : null
  if (isUnavailableError(mutation.error))
    return <UnavailableRedirect error={mutation.error} />

  function open(action: Action) {
    mutation.reset()
    setNotice('')
    setPending(action)
  }

  function cancel() {
    if (!mutation.isPending) setPending(undefined)
  }

  return (
    <section aria-label="Блокировка и сессии" className="cp-users__panel">
      <div className="cp-users__controls">
        {statusAction && (
          <Button
            ref={statusButton}
            size="xl"
            view="outlined"
            disabled={!statusCurrent}
            onClick={() => {
              open(statusAction)
            }}
          >
            {actions[statusAction].label}
          </Button>
        )}
        <Button
          ref={sessionsButton}
          size="xl"
          view="outlined"
          disabled={!statusCurrent}
          onClick={() => {
            open('sessions')
          }}
        >
          {actions.sessions.label}
        </Button>
      </div>
      {notice && (
        <p role="status">
          <Icon data={CircleCheck} size={16} aria-hidden="true" /> {notice}
        </p>
      )}
      {pending && (
        <DialogFrame
          title={actions[pending].label}
          description={userName(user) + '. ' + actions[pending].description}
          focusSafeAction
          openerRef={pending === 'sessions' ? sessionsButton : statusButton}
          onCancel={cancel}
          actions={
            <>
              <Button
                size="xl"
                view="outlined"
                disabled={mutation.isPending}
                onClick={cancel}
              >
                Отмена
              </Button>
              <Button
                size="xl"
                view={pending === 'unblock' ? 'action' : 'outlined-danger'}
                disabled={mutation.isPending}
                onClick={() => {
                  mutation.mutate(pending)
                }}
              >
                {actions[pending].label}
              </Button>
            </>
          }
        >
          {mutation.isPending && (
            <p role="status">
              <Icon data={Clock} size={16} aria-hidden="true" /> Выполняется
              запрос
            </p>
          )}
          {mutation.isError && (
            <ErrorState reason="Не удалось выполнить действие. Повторите запрос." />
          )}
        </DialogFrame>
      )}
    </section>
  )
}
