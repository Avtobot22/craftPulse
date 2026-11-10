# Ведение проекта в GitHub

Issue хранит задачу, Projects показывает очередь и текущую работу, PR содержит реализацию и ревью. Документация продукта остаётся в Git рядом с кодом. Одну задачу не нужно дублировать в другом трекере.

## Доска

Доска [craftPulse](https://github.com/users/Avtobot22/projects/4) связана с репозиторием `Avtobot22/craftPulse`. Доступ к репозиторию сам по себе не даёт права редактировать Projects. Участникам разработки нужен отдельный доступ Write к проекту.

| Статус | Значение |
| --- | --- |
| Backlog | Задача зарегистрирована, но ещё не подготовлена |
| Ready | Есть границы, приёмка и способ проверки; зависимости позволяют начать |
| In progress | Исполнитель делает задачу |
| In review | Результат готов и ожидает ревью или приёмки |
| Done | Issue завершена после проверки результата |

Одна карточка на задачу; PR в доску повторно не добавляем. Фильтр рабочей доски исключает `type:epic`, чтобы родитель не дублировал дочерние задачи. Эпики видны в отдельном представлении. Метки `type:*` обозначают вид работы, `area:*` - направление, `priority:*` - оперативный порядок задач. Отдельный срок нужен только при реальном обязательстве.

## Оценка и бизнес-приоритет

| Поле Projects | Для чего | Значения |
| --- | --- | --- |
| Story points | Размер задачи, функции или ошибки | Число по шкале `1, 2, 3, 5, 8, 13`; пусто - ещё не оценена |
| Бизнес-приоритет | Очерёдность эпика по ценности продукта и поставке | `Высокий`, `Обычный`, `Низкий`; пусто - ещё не определён |

`Story points` показано в карточках `Работа` и таблице `Очередь`, `Бизнес-приоритет` - в `Эпики` и `Очередь`. Перед переводом задачи в `Ready` исполнитель и команда согласуют SP; задачу больше 13 SP делят. Ответственный за продукт вместе с командой определяет бизнес-приоритет эпика и объясняет его в описании. У эпика SP пустые; у дочерней задачи бизнес-приоритет пустой. Оценки не переводятся в часы и не сравнивают участников.

Поля заполняются на доске или в разделе проекта внутри Issue. Issue forms не записывают значения в Projects автоматически. Workflow `Project readiness` проверяет наличие SP перед автоматическим переводом из `Backlog` в `Ready`; ручной перевод остаётся ответственностью команды. Неизвестные оценки оставляйте пустыми. Просмотреть имена, типы и ID полей можно командой `gh project field-list 4 --owner Avtobot22`.

## Автоматизация

В Projects используются встроенные workflows:

- Auto-add to project: репозиторий `Avtobot22/craftPulse`, фильтр `is:issue is:open`.
- Item added to project: `Backlog`.
- Item closed: `Done`.
- Pull request linked to issue: `In progress` для связанной задачи.

Auto-add подхватывает новые и изменённые задачи после включения. Уже существующие задачи нужно добавить отдельно. Дочерние задачи добавляются через Auto-add sub-issues to project. Автоматическое закрытие Issue при переносе карточки в Done выключено: готовность подтверждается закрытием самой Issue. Исполнитель переводит задачу в `In progress` в начале работы, не дожидаясь PR, и в `In review`, когда результат готов к необязательному ревью или приёмке. При повторном открытии Issue верни карточку в `Backlog` и проверь готовность заново. Блокировки отражаются через Relationships, без второго независимого списка.

Встроенные workflows, вид доски и фильтры настраиваются в интерфейсе Projects. Они не создаются файлами из `.github/`. Правила защиты, настройки репозитория и labels хранятся в JSON, чтобы изменения можно было ревьюить и восстановить.

### Синхронизация Backlog и Ready

Workflow `.github/workflows/project-readiness.yml` запускает `scripts/github_project_readiness.py` после закрытия и повторного открытия Issue этого репозитория, каждый час на 17-й минуте UTC и вручную через Actions → Project readiness → Run workflow. Он читает код основной ветки и не запускается на PR. Расписание служит также для изменений SP, меток и связей, у которых нет отдельного триггера в этом workflow. GitHub может задержать или пропустить запуск по расписанию; строгой гарантии обработки за час нет. В публичном репозитории GitHub отключает расписание после 60 дней без активности.

Скрипт находит открытую пользовательскую доску `craftPulse` владельца `Avtobot22`, поля `Status` и `Story points`, варианты `Backlog` и `Ready` по именам через GraphQL API. ID не хранятся в коде. Если одноимённых открытых досок несколько, поля отсутствуют или API недоступен, запуск завершается ошибкой. Связи читаются через REST `repos/Avtobot22/craftPulse/issues/{number}/dependencies/blocked_by`, включая все страницы ответа.

- Открытая Issue из `Avtobot22/craftPulse` в `Backlog` переходит в `Ready`, если `Story points` заполнено, нет метки `type:epic` и все блокеры закрыты. Отсутствие блокеров также удовлетворяет условию.
- Открытая Issue в `Ready` возвращается в `Backlog`, если есть открытый блокер. Удаление SP само по себе не возвращает задачу в `Backlog`.
- `In progress`, `In review`, `Done`, закрытые Issue, архивные карточки, PR, черновики и задачи других репозиториев пропускаются. Меняется только `Status`.

Это проверка формальных условий очереди. Полноту описания, критерии приёмки и согласование оценки по-прежнему проверяет команда. Перед записью скрипт повторно читает карточку, SP, метки и блокеры, чтобы учесть начавшуюся работу или новый блокер. Запуски workflow выполняются последовательно; ручное изменение между последним чтением и записью остаётся возможным, поскольку API не предоставляет условную запись статуса. После записи скрипт проверяет результат. Ошибка останавливает дальнейшие изменения; уже выполненные переходы не откатываются, повторный запуск проверяет актуальное состояние.

Владелец добавляет Actions secret репозитория `PROJECT_TOKEN` в Settings → Secrets and variables → Actions. Нужен personal access token **classic** пользователя с правом Write на доску: scope `project` для чтения и записи Projects, а для чтения приватных Issues и их блокеров также `repo`. Токен должен видеть все связанные приватные репозитории. Ошибки чтения связей останавливают запуск и не считаются отсутствием блокеров. Fine-grained PAT не поддерживает Projects пользовательского аккаунта; встроенный `GITHUB_TOKEN` к этой доске доступа не имеет. Проверено по документации GitHub 2026-11-10: [права автоматизации Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/automating-projects-using-actions), [ограничения fine-grained PAT](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#fine-grained-personal-access-tokens-limitations), [REST для блокирующих связей](https://docs.github.com/en/rest/issues/issue-dependencies#list-dependencies-an-issue-is-blocked-by), [ограничения расписания](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule).

Без `PROJECT_TOKEN` workflow останавливается с понятной ошибкой до checkout и запросов API. Токен передаётся только через окружение шага; скрипт не выводит его, заголовки авторизации или содержимое ошибок API. У `GITHUB_TOKEN` остаётся только `contents: read`, checkout не сохраняет credentials, actions закреплены полным SHA.

Для локальной проверки нужен Python 3.9+ и заранее установленный в окружении `PROJECT_TOKEN`; не записывайте значение в команды, файлы и журналы. По умолчанию скрипт только показывает план:

```sh
python3 scripts/github_project_readiness.py
```

Запись включается явно:

```sh
python3 scripts/github_project_readiness.py --apply
```

После добавления секрета и слияния workflow проверьте на тестовых Issue A и B: задайте A оценку и блокер B, закройте B и дождитесь `Ready` у A. Добавьте новый открытый блокер и запустите workflow вручную: A должна вернуться в `Backlog`. Переведите A в `In progress` и повторите запуск: статус должен сохраниться. Проверьте журнал на отсутствие токена, затем закройте тестовые Issue как `not planned`.

## Настройки репозитория

Файлы конфигурации:

- `.github/repository-settings.json` - squash merge, удаление рабочих веток и формат сообщения коммита.
- `.github/main-ruleset.json` - правила `main` без обхода для администраторов.
- `.github/labels.json` - управляемые метки. Остальные метки скрипт сохраняет.
- `.github/CODEOWNERS` - запрос ревью владельцу по умолчанию.
- `.github/workflows/repository.yml` - обязательный `repository-check` на каждом PR и после merge в `main`.
- `.github/dependabot.yml` - еженедельные PR обновлений GitHub Actions.
- `.github/release.yml` - группировка изменений в GitHub Release.

У GitHub Actions минимальные права: чтение содержимого, без автоматического одобрения PR и без сохранённых credentials в checkout. Сторонние actions закрепляются полным commit SHA; обновление проходит обычный PR. В CI не используется привилегированный `pull_request_target` и не передаются секреты коду PR.

## Проверка и восстановление

Нужны Python 3.9+, GitHub CLI и права администратора `Avtobot22/craftPulse`. `gh` должен быть авторизован на `github.com`. Для Projects дополнительно нужен scope `project`.

Команды по умолчанию только показывают отличие или целевую конфигурацию:

```sh
python3 scripts/github_setup.py repository
python3 scripts/github_setup.py labels
python3 scripts/github_setup.py protection
```

Применение:

```sh
python3 scripts/github_setup.py repository --apply
python3 scripts/github_setup.py labels --apply
python3 scripts/github_setup.py protection --apply
```

Защита требует предварительного успешного прогона Repository CI, чтобы обязательная проверка уже существовала. При первом подключении сначала открой PR с workflow, дождись зелёного прогона, затем включи защиту и выполни обычное ревью. Скрипт обновляет только именованный ruleset `craftPulse main protection`; другие правила сохраняются. Ошибка прав, тарифа или проверки завершается ненулевым кодом, без заявления об успехе.

После применения проверь в Settings → Rules → Rulesets, что правило активно для `main`, обходы отсутствуют, требуется `repository-check`, а обязательное одобрение другим участником выключено. Защита приватных репозиториев зависит от тарифа GitHub. При ошибке тарифа защита не включена; не делай репозиторий публичным ради обхода этого ограничения.

## Релизы

Milestone объединяет задачи ближайшего поставляемого результата. При публикации создай тег на проверенном коммите и GitHub Release с автоматически собранными заметками. Метки PR управляют категориями заметок. Пока исполняемого продукта нет, автоматическую публикацию и фиктивные версии не создаём. Первый релиз приложения должен включить воспроизводимую сборку и проверку артефакта; наличие тега само по себе этого не доказывает.

## Документация GitHub

- [Sub-issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues)
- [Блокирующие зависимости](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/creating-issue-dependencies)
- [Встроенные workflows Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-built-in-automations)
- [Автоматическое добавление задач](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/adding-items-automatically)
- [Rulesets и доступность по тарифам](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets)
- [Безопасность GitHub Actions](https://docs.github.com/en/actions/reference/security/secure-use)
