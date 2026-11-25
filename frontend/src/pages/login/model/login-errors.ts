const errors = {
  provider_unavailable: {
    title: 'GitLab недоступен',
    message: 'Попробуйте войти позже.',
    action: 'Повторить вход',
    retry: true,
  },
  provider_rejected: {
    title: 'GitLab отказал во входе',
    message:
      'GitLab отклонил вход или регистрацию приложения. Обратитесь к его администратору.',
    action: 'Войти снова',
    retry: true,
  },
  invalid_response: {
    title: 'Не удалось проверить ответ GitLab',
    message: null,
    action: 'Начать вход заново',
    retry: true,
  },
  user_blocked: {
    title: 'Учётная запись заблокирована в craftPulse',
    message: 'Обратитесь к администратору craftPulse.',
    action: 'Вернуться к началу',
    retry: false,
  },
  organization_not_configured: {
    title: 'Организация ещё не настроена',
    message: 'Обратитесь к администратору craftPulse.',
    action: 'Вернуться к началу',
    retry: false,
  },
} as const

const unknownError = {
  title: null,
  message: 'Если ошибка повторяется, обратитесь к администратору craftPulse.',
  action: 'Начать вход заново',
  retry: true,
} as const

export function loginErrorMessage(code: string) {
  switch (code) {
    case 'provider_unavailable':
      return errors.provider_unavailable
    case 'provider_rejected':
      return errors.provider_rejected
    case 'invalid_response':
      return errors.invalid_response
    case 'user_blocked':
      return errors.user_blocked
    case 'organization_not_configured':
      return errors.organization_not_configured
    default:
      return unknownError
  }
}
