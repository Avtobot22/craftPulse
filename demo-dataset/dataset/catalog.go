package dataset

type Variant struct {
	ID               string   `json:"id"`
	Sources          []string `json:"sources"`
	Representation   string   `json:"representation"`
	ExpectedBehavior string   `json:"expected_behavior"`
	VerificationSets []string `json:"verification_sets"`
}

func Catalog() []Variant {
	return []Variant{
		{ID: "created-active", Sources: []string{"jira_cloud", "universal"}, Representation: "Задача создана в подтверждённом рабочем статусе", ExpectedBehavior: "Начало равно created", VerificationSets: []string{"V-14"}},
		{ID: "intake-active-category", Sources: []string{"jira_cloud"}, Representation: "Входящий статус с API-category active", ExpectedBehavior: "Начала нет до перехода в рабочий статус", VerificationSets: []string{"V-14"}},
		{ID: "blocked", Sources: []string{"jira_cloud", "universal"}, Representation: "Явные интервалы blocked внутри active, в том числе перекрывающиеся блокировки по флагу и статусу", ExpectedBehavior: "Блокировка не меняет начало; перекрытия объединяются; интервалы раскрываются", VerificationSets: []string{"V-03", "V-41"}},
		{ID: "cancelled", Sources: []string{"jira_cloud", "universal"}, Representation: "Терминальный исход cancelled", ExpectedBehavior: "Не включать в successful completion", VerificationSets: []string{"V-03"}},
		{ID: "duplicate", Sources: []string{"jira_cloud"}, Representation: "Терминальный исход duplicate", ExpectedBehavior: "Не включать в successful completion", VerificationSets: []string{"V-03"}},
		{ID: "done-reopen-done", Sources: []string{"jira_cloud", "universal"}, Representation: "Два завершения с переоткрытием между ними", ExpectedBehavior: "Сохранить эпизоды; reopen не production defect", VerificationSets: []string{"V-02", "V-03"}},
		{ID: "size-change", Sources: []string{"jira_cloud"}, Representation: "S на start, M после start", ExpectedBehavior: "Сохранить размер на старте и версию target", VerificationSets: []string{"V-16"}},
		{ID: "team-change", Sources: []string{"jira_cloud"}, Representation: "Владение проектом A до границы и B после", ExpectedBehavior: "M0 помечает current grouping, история не выдумывается", VerificationSets: []string{"V-11"}},
		{ID: "unknown-status", Sources: []string{"jira_cloud"}, Representation: "Переход в статус без правила mapping", ExpectedBehavior: "Статус перечислен как unmapped, не даёт начала и завершения", VerificationSets: []string{"V-14"}},
		{ID: "old-open", Sources: []string{"jira_cloud"}, Representation: "Открытая задача без недавнего updated", ExpectedBehavior: "Входит в open backlog независимо от updated", VerificationSets: []string{"V-06"}},
		{ID: "incomplete-history-page", Sources: []string{"jira_cloud"}, Representation: "Сбой второй страницы истории при page_size 10", ExpectedBehavior: "Нет полного watermark; восстановление без дублей", VerificationSets: []string{"V-05", "V-06"}},
		{ID: "same-key-instances", Sources: []string{"jira_cloud", "universal"}, Representation: "Один отображаемый ключ в двух instances", ExpectedBehavior: "Две identity, не автоматическая связь", VerificationSets: []string{"V-17"}},
		{ID: "bot-response", Sources: []string{"gitlab", "github"}, Representation: "Bot и author комментируют до внешнего human response", ExpectedBehavior: "Bot и author не завершают human clock", VerificationSets: []string{"V-15"}},
		{ID: "ambiguous-link", Sources: []string{"gitlab", "github", "jira_cloud"}, Representation: "Ключ без instance в имени ветки", ExpectedBehavior: "Только candidate; нет подтверждённой причины задержки", VerificationSets: []string{"V-17"}},
		{ID: "head-without-scan", Sources: []string{"gitlab", "github"}, Representation: "Новый head SHA без scan", ExpectedBehavior: "NOT_EVALUATED, не clean", VerificationSets: []string{"V-01", "V-24"}},
		{ID: "report-other-sha", Sources: []string{"gitlab", "github", "jenkins", "sonarqube"}, Representation: "Отчёт для h0 при текущем head h1", ExpectedBehavior: "Чужой SHA не доказательство чистоты h1", VerificationSets: []string{"V-24", "V-25"}},
		{ID: "multi-period-history", Sources: []string{"jira_cloud", "universal"}, Representation: "Полные start/completion минимум 20 задач за несколько недель и месяцев", ExpectedBehavior: "Когорты совпадают с периодом и масштабом; неполный период виден", VerificationSets: []string{"V-03", "V-08"}},
		{ID: "types-sizes-targets", Sources: []string{"jira_cloud", "universal"}, Representation: "Разные типы, S/M, unknown size/target и версии target v1/v2", ExpectedBehavior: "Неизвестные группы видны, новая версия не переписывает старое нарушение", VerificationSets: []string{"V-14", "V-16"}},
		{ID: "complete-reopen-followup", Sources: []string{"jira_cloud", "universal"}, Representation: "Завершение с полностью наблюдаемым окном reopen", ExpectedBehavior: "Входит в eligible denominator T23", VerificationSets: []string{"V-03"}},
		{ID: "incomplete-reopen-followup", Sources: []string{"jira_cloud", "universal"}, Representation: "Завершение близко к as-of, окно reopen ещё не завершено", ExpectedBehavior: "Неполное окно видно; не считать отсутствие reopen доказанным", VerificationSets: []string{"V-03"}},
		{ID: "late-event", Sources: []string{"jira_cloud", "universal"}, Representation: "event_time раньше observed_time, доставка после первой ревизии", ExpectedBehavior: "Новая ревизия уточняет результат, прежняя неизменна", VerificationSets: []string{"V-02", "V-05"}},
		{ID: "replay", Sources: []string{"jira_cloud", "gitlab", "github", "universal"}, Representation: "Три доставки одинаковых устойчивых IDs", ExpectedBehavior: "Один объект, событие и вклад", VerificationSets: []string{"V-02"}},
	}
}
