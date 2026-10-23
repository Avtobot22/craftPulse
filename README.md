# craftPulse

Платформа аналитики разработки: связывает задачи, изменения кода, CI и находки анализаторов, показывает ролевые дашборды с раскрытием до исходных событий.

## Состояние проекта

В репозитории находятся продуктовые документы и каркас будущих модулей. Запускаемого приложения пока нет. Планируемый стек - Go, React + TypeScript и PostgreSQL; команды запуска и сборки появятся вместе с реализацией.

## Проверка репозитория

Нужны Git и Python 3.9+ без дополнительных пакетов:

```sh
python3 scripts/check_repository.py
python3 -m unittest discover -s tests -v
git diff --check
```

После клонирования подключи локальную проверку заголовка коммита:

```sh
git config --local core.hooksPath .githooks
```

## Документация и работа команды

- [Документация продукта](docs/README.md)
- [Видение продукта](docs/product/vision.md)
- [Метрики](docs/product/metrics.md)
- [Работа над задачами и PR](CONTRIBUTING.md)
- [Доска, настройки и автоматизация GitHub](docs/github-workflow.md)
- [Автоматизация зависимостей с Copilot](docs/copilot-automation.md)
- [Секреты и уязвимости](SECURITY.md)
- [Доска craftPulse](https://github.com/users/Avtobot22/projects/4)
