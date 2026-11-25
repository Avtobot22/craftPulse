import { Button } from '@gravity-ui/uikit'
import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { useBlocker } from 'react-router'
import type { Blocker } from 'react-router'
import { routePaths } from '@/shared/config'
import { DialogFrame } from './dialog-frame'

export interface UnsavedChangesDialogProps {
  formName: string
  formRef: RefObject<HTMLFormElement | null>
  dirty: boolean
  /** Defer the navigation prompt until the confirmation dialog has closed. */
  confirmationOpen?: boolean
  hasSensitiveInput?: boolean
  /** Clear the local draft before allowing navigation. */
  onDiscard: () => void
}

export function UnsavedChangesDialog({
  formName,
  formRef,
  dirty,
  confirmationOpen = false,
  hasSensitiveInput = false,
  onDiscard,
}: UnsavedChangesDialogProps) {
  const lastFormFocus = useRef<HTMLElement | null>(null)
  const blocker = useBlocker(({ nextLocation }) => {
    const path = nextLocation.pathname
    // The session and access guards are allowed to remove protected content.
    return (
      dirty &&
      path !== routePaths.sessionExpired &&
      path !== routePaths.unavailable
    )
  })

  useEffect(() => {
    const form = formRef.current
    if (!form) return
    function rememberFocus(event: FocusEvent) {
      if (event.target instanceof HTMLElement)
        lastFormFocus.current = event.target
    }
    form.addEventListener('focusin', rememberFocus)
    return () => {
      form.removeEventListener('focusin', rememberFocus)
    }
  }, [formRef])

  useEffect(() => {
    if (!dirty) return
    function warnBeforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault()
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => {
      window.removeEventListener('beforeunload', warnBeforeUnload)
    }
  }, [dirty])

  useEffect(() => {
    if (!dirty && blocker.state === 'blocked') blocker.reset()
  }, [dirty, blocker])

  if (!dirty || confirmationOpen || blocker.state !== 'blocked') return null

  return (
    <UnsavedPrompt
      blocker={blocker}
      formName={formName}
      hasSensitiveInput={hasSensitiveInput}
      lastFormFocus={lastFormFocus}
      onDiscard={onDiscard}
    />
  )
}

function UnsavedPrompt({
  blocker,
  formName,
  hasSensitiveInput,
  lastFormFocus,
  onDiscard,
}: {
  blocker: Extract<Blocker, { state: 'blocked' }>
  formName: string
  hasSensitiveInput: boolean
  lastFormFocus: RefObject<HTMLElement | null>
  onDiscard: () => void
}) {
  const discarding = useRef(false)

  function discard() {
    if (discarding.current) return
    discarding.current = true
    onDiscard()
    blocker.proceed()
  }

  return (
    <DialogFrame
      title="Изменения не сохранены"
      description={`В форме «${formName}» есть несохранённые изменения.`}
      focusSafeAction
      onCancel={blocker.reset}
      returnFocus={(opener) =>
        discarding.current ? false : (lastFormFocus.current ?? opener)
      }
      actions={
        <>
          <Button view="action" size="xl" onClick={blocker.reset}>
            Продолжить редактирование
          </Button>
          <Button view="outlined" size="xl" onClick={discard}>
            Отбросить изменения
          </Button>
        </>
      }
    >
      {hasSensitiveInput && (
        <p>После ухода секретные значения нельзя будет восстановить.</p>
      )}
    </DialogFrame>
  )
}
