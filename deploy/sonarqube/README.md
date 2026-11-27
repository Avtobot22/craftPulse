# Сверка анализов Go на стенде SonarQube

Используйте отдельный тестовый проект и токен сканера с правом Execute Analysis. Для чтения нужен токен с правом Browse. Передайте сканеру `SONAR_TOKEN`, а скрипту записи `SONAR_READ_TOKEN` через окружение оператора.

Сгенерируйте fixture-репозиторий `orders-service` по инструкции [набора показа](../../demo-dataset/README.md#репозитории-и-git-документы). При генерации с `--seed 21` и `--repo-dir "$work/repos"` рабочую копию и два последних SHA ветки `main` можно получить так.

```sh
git clone --branch main "$work/repos/repos/org-1/gitlab-1/repo-1.git" "$work/orders-service"
git -C "$work/orders-service" rev-list --max-count=2 --reverse main
```

Для каждого SHA по порядку от старого к новому выполните команды из корня рабочей копии.

```sh
git switch --detach "$fixture_sha"
sonar-scanner -Dsonar.host.url="$sonar_url" -Dsonar.projectKey="$sonar_project" \
  -Dsonar.scm.revision="$fixture_sha" -Dsonar.sources=. -Dsonar.qualitygate.wait=true
python3 "$craftpulse/deploy/sonarqube/record_fixture.py" --url "$sonar_url" \
  --project "$sonar_project" --revision "$fixture_sha" \
  --out "$craftpulse/deploy/sonarqube/fixture-analyses.json"
```

Дождитесь успешной обработки каждого анализа сервером. Если Quality Gate отклонил результат, проверьте, что задача Compute Engine завершилась успешно и анализ получил ключ. Скрипт сохраняет снимок текущих issues до следующего анализа. Он читает Web API напрямую и сверяет ревизию с SHA checkout.

После второго анализа запросите `Probe`, `Manifest` и `Fetch` по проекту. Сверьте оба ключа, время и SHA с `fixture-analyses.json`. Для последнего анализа сравните число и набор ключей issues, правила, пути и диапазоны строк. Для первого анализа проверьте метаданные. JSON с результатами храните рядом с `instance.json`.

## Локальная сверка

[Запись локального прогона](fixture-analyses.local.json) содержит два анализа fixture из набора показа с seed 21. SonarQube Community Build 26.11.0.134175 запущен из образа, закреплённого в `instance.json`. Чтобы получить непустой перечень находок, для тестового проекта создана копия штатного профиля Go с правилом `go:S3776` и параметром `threshold=0`. Сервер вернул по 15 issues на каждой ревизии.

Тест `TestSonarQubeLocalEvidence` воспроизводит ответы Web API и сверяет ключи, время и SHA обоих анализов с независимой записью. Для последнего анализа он сверяет ключи, правила, пути, строки и исходное состояние всех 15 находок и проверяет наличие шести метрик. Записанные ответы содержат только поля, которые читает адаптер. Многостраничная выдача проверяется на синтетических ответах.

На локальных частных проектах пользовательский токен получил HTTP 200 при наличии Browse и HTTP 403 при отсутствии и после отзыва Browse на `/api/components/show` и `/api/project_analyses/search`. Поиск issues в обоих случаях отказа вернул HTTP 200 с пустым списком.
