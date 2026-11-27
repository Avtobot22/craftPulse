import { Button } from '@gravity-ui/uikit'
import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { useBlocker } from 'react-router'
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
  allowSearchChanges?: boolean
  discardRequested?: boolean
  onKeepEditing?: () => void
}

export function UnsavedChangesDialog({
  formName,
  formRef,
  dirty,
  confirmationOpen = false,
  hasSensitiveInput = false,
  onDiscard,
  allowSearchChanges = false,
  discardRequested = false,
  onKeepEditing,
}: UnsavedChangesDialogProps) {
  const lastFormFocus = useRef<HTMLElement | null>(null)
  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    const path = nextLocation.pathname
    // The session and access guards are allowed to remove protected content.
    return (
      dirty &&
      !(allowSearchChanges && currentLocation.pathname === path) &&
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

  if (
    !dirty ||
    confirmationOpen ||
    (!discardRequested && blocker.state !== 'blocked')
  )
    return null

  return (
    <UnsavedPrompt
      onKeepEditing={
        blocker.state === 'blocked' ? blocker.reset : onKeepEditing
      }
      onDiscard={() => {
        onDiscard()
        if (blocker.state === 'blocked') blocker.proceed()
      }}
      formName={formName}
      hasSensitiveInput={hasSensitiveInput}
      lastFormFocus={lastFormFocus}
    />
  )
}

function UnsavedPrompt({
  onKeepEditing,
  formName,
  hasSensitiveInput,
  lastFormFocus,
  onDiscard,
}: {
  onKeepEditing: (() => void) | undefined
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
  }

  return (
    <DialogFrame
      title="Изменения не сохранены"
      description={`В форме «${formName}» есть несохранённые изменения.`}
      focusSafeAction
      onCancel={() => onKeepEditing?.()}
      returnFocus={(opener) =>
        discarding.current ? false : (lastFormFocus.current ?? opener)
      }
      actions={
        <>
          <Button view="action" size="xl" onClick={onKeepEditing}>
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
