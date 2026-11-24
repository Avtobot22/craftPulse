# Тема и общие состояния craftPulse

Приложение использует UIKit 8.1.0 и его штатные компоненты. `Provider` задаёт тему, язык, layout и defaultProps. `theme.css` генерируется официальным `@gravity-ui/uikit-themer` 2.1.0; исходные значения находятся в `tools/generate-theme.mjs`. Публичные переменные компонентов находятся в `controls.css`. Внутренние CSS-классы UIKit не переопределяются.

Inter поставляется локально. Копирование выполняет ClipboardButton, всплывающие сообщения показывает Toaster, компоновку выполняют Box/Flex/Container/Grid. Общие состояния в `@/shared/ui` добавляют только подписи, безопасную диагностику и состав результата по [ТЗ](../../../../docs/technical-specification/technical-specification.md).

```bash
npm run generate:theme
npm run check:theme
```

Сборка проверяет соответствие theme.css генератору. Генератор разделяет brand и warning: в исходной палитре Themer они используют yellow; для акцента создаётся отдельная brand-палитра. Для HC применяются те же изменённые семантические роли, а базовые HC-поверхности сохраняются из UIKit.

## Соответствие DS и реализации

| Токен | Штатный механизм                                                    | Настройка                                                                                                                                                                                                                                               |
| ----- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| DS-01 | Themer `typography.fontFamilies`; локальный `@fontsource/inter`     | Inter 400/600, monospace fallback. Внешний fonts.css не импортируется; числа таблиц tabular.                                                                                                                                                            |
| DS-02 | Themer `typography.variants`, `Text`                                | H1 28/36, H2 24/32, H3 20/28, body 16/24, caption/code 14/20, display 32/40. Размеры в rem.                                                                                                                                                             |
| DS-03 | `--g-color-base-*`, `--g-color-text-*`, `--g-color-line-*`          | Семантические роли UIKit. Вторичный текст и значимые границы заданы в генераторе по таблице контраста.                                                                                                                                                  |
| DS-04 | Themer `updateBaseColor`, brand/link/selection utility colors       | Брендовая палитра генерируется целиком. Ссылки подчёркнуты и поддерживают visited через настройки Link. Для SPA-переходов Link используется с useLinkClickHandler React Router.                                                                         |
| DS-05 | Семантические пары Themer; `Alert`, `Label`                         | Positive/warning/danger/info с текстом и иконкой. Для UNKNOWN применяется нейтральная тема Label.                                                                                                                                                       |
| DS-06 | `--g-spacing-*`, `--g-border-radius-*`, `--g-color-sfx-*`           | Штатные отступы с базой 4 px, радиусы и тени UIKit; отступы композиций задаются props Box/Flex.                                                                                                                                                         |
| DS-07 | `Provider.layout`, `Container`, `Grid`, `layoutSizes`               | Breakpoints 768/1280; сетка 4/8/12 колонок, gap 16/16/24, padding 16/24/32. layoutSizes: content=1600px, navigation=240px, form=720px, reading=80ch. Полная оболочка UI-01 реализуется в #264.                                                          |
| DS-08 | `--g-color-line-focus`, публичные focus-переменные UIKit            | Светлый фокус #7C3AED, тёмный #C4B5FD. Кнопка и Link используют offset 2 px; поля и выбор сохраняют штатное расположение кольца.                                                                                                                        |
| DS-09 | `Provider.defaultProps.Button`, `--g-button-*`                      | Размер xl даёт высоту 44 px. Шрифт 14 px, вес 600, padding 12, gap 8. Поведение и геометрия иконок остаются у Button.                                                                                                                                   |
| DS-10 | `Provider.defaultProps.TextInput/TextArea`                          | Размер xl; textarea minRows=5 с ростом. Границы и фокус настроены публичными CSS-переменными.                                                                                                                                                           |
| DS-11 | `Select size=xl`, `popupClassName`, `--g-list-item-view-min-height` | Поле и строки выбора от 44 px. Публичный popupClassName ограничивает список до min(320px, 100dvh - 32px). Календарь этой задачей не устанавливается.                                                                                                    |
| DS-12 | `Checkbox/Radio size=l`, публичный style                            | Цель от 44×44 через style; индикаторы штатные 18 px. Отдельной перерисовки control нет.                                                                                                                                                                 |
| DS-13 | Штатные компоненты таблиц, `Text`                                   | Общая типографика и tabular nums; прикладные таблицы в эту задачу не входят.                                                                                                                                                                            |
| DS-14 | `Pagination`, `Button` UIKit                                        | Пагинацию и курсор задаёт экран; нового общего компонента пагинации нет.                                                                                                                                                                                |
| DS-15 | `Flex`, `Select`, `Label`, `Button`                                 | Фильтры собираются из UIKit; бизнес-правила применения и сброса принадлежат экрану.                                                                                                                                                                     |
| DS-16 | `Card`, `Text`, `Label`, layout props                               | Карточка показателя собирается на экране. Отдельного DataValue-компонента нет.                                                                                                                                                                          |
| DS-17 | Цветовые токены текущей темы                                        | Графики реализуются отдельными задачами; библиотека графиков этой задачей не добавляется.                                                                                                                                                               |
| DS-18 | `Tab`, `Breadcrumbs` UIKit                                          | Используются напрямую; их поведение и размеры не переопределяются внутренними селекторами.                                                                                                                                                              |
| DS-19 | `Dialog`, `Modal` UIKit                                             | Фокус, Portal, Escape, возврат и геометрия штатные. Диалоги UI-17/UI-18 вне задачи.                                                                                                                                                                     |
| DS-20 | `ToasterProvider`, `ToasterComponent`, `useToaster`                 | useNotifications задаёт иконки и политику: успех 8000 мс, ошибка и сообщение с действием без таймера. Portal, анимация, закрытие и hover принадлежат UIKit.                                                                                             |
| DS-21 | `Label`, `Icon`, `Text`                                             | DataStateBadge задаёт соответствие шести S-05/S-06 текстам, теме и иконке.                                                                                                                                                                              |
| DS-22 | `Card`, `Text`, `ClipboardButton`                                   | Полный SHA с overflow-wrap:anywhere. Буфер, fallback и результат выполняет UIKit; приложение объявляет результат и исключает скрытое значение.                                                                                                          |
| DS-23 | `Skeleton`, `Spin`, `Box`, `Flex`                                   | Skeleton задаёт геометрию; статус «Загрузка» расположен поверх него и не увеличивает высоту. При minHeight=320px и блоке 240px внешний размер остаётся 320px; значение по умолчанию 160px.                                                              |
| DS-24 | `PlaceholderContainer`, `Alert`                                     | Пустой результат: padding 24, icon 24, заголовок 16/24, описание 14/20. Полноразмерная ошибка: padding 24, icon 24, заголовок 16/24, текст 14/20 до 80ch. Безопасные причины, correlation_id и повтор; встроенный успех и устаревание используют DS-20. |
| DS-25 | Штатные Portal, LayerManager, transitions и reduced motion UIKit    | Внутренние слои и анимации библиотеки не переопределяются.                                                                                                                                                                                              |
| DS-26 | `Box`, `Flex`, `Container` и native controls                        | Структура формы принадлежит экрану; общих CSS-классов формы и повторных обёрток полей нет.                                                                                                                                                              |
| DS-27 | `Tooltip`, `Provider.defaultProps.Tooltip`, `--g-tooltip-padding`   | Hover 300 мс, focus без задержки; offset 8 px, текст 14/20, padding 8/12, ширина min(320px, 100vw - 32px). Управление и Portal принадлежат UIKit.                                                                                                       |

## Состояния и безопасные данные

ErrorState получает подготовленную безопасную причину и отдельно correlationId. Problem.detail, credentials и названия недоступных объектов передавать в reason нельзя. retrying блокирует повтор до завершения текущего запроса; ввод и параметры остаются у экрана.

StaleState получает только разрешённый снимок. Дата снимка и текущая ошибка отображаются раздельно. TechnicalValue с hidden не принимает value и не создаёт действие копирования. ClipboardButton сообщает результат через onCopy; приложение объявляет его в status/alert.

useNotifications использует штатный useToaster. Короткое сообщение передаётся в content как Text caption-1 (14/20), без заголовка h3. Контурные иконки CircleXmark и CircleCheck передаются через renderIcon штатного Toaster; их расположение задаёт UIKit. name идентифицирует сообщение; повтор с тем же name обрабатывает библиотека. Успех исчезает через 8 секунд, ошибка и сообщение с actions остаются до закрытия. Постоянный результат операции показывается в исходной форме через SuccessState.

EmptyState передаёт действия массивом в PlaceholderContainer: библиотека создаёт кнопки и отступ 20 px после описания. ErrorState использует Alert.Actions и Alert.Action. Его полноразмерный блок получает параметры DS-24 через класс приложения и публичные переменные Alert. Встроенные Alert сохраняют параметры DS-20.

В ошибке и тосте используется контурный CircleXmark, в успехе CircleCheck. Полноразмерная ошибка имеет иконку 24 px, тост и встроенное сообщение 20 px. Семантические цвета одинаковы: danger-heavy и positive-heavy. Для Alert задан verticalAlign=middle через публичный style Icon; другие компоненты сохраняют штатное выравнивание. DataStateBadge задаёт текст, тему и иконку штатного Label; необязательная причина остаётся видимым текстом рядом с ним.

## Отличия штатного поведения от отдельных размеров DS

UIKit Toaster 8.1.0 приостанавливает таймер при hover и после ухода запускает полный интервал заново. Кнопка закрытия Toaster штатного размера s имеет цель 24×24 px. Клавиатурный фокус таймер не останавливает; стандартный контейнер использует aria-live=assertive для всех уведомлений. Успех и ошибка не получают отдельных ролей status/alert в Toaster. Встроенные сообщения и результат копирования сохраняют эти роли.

Поля и выбор сохраняют штатную геометрию фокуса; активный пункт Menu выделяется фоном без внешнего кольца 2 px. Индикаторы Checkbox/Radio размера l имеют 18 px внутри цели от 44×44 px; их типографика остаётся штатной. Толщина ошибочной границы поля остаётся штатной.

Размеры вкладок и диалогов, времена переходов и слои также остаются штатными. Popup Select и Tooltip ограничены через публичные настройки. Spin при reduced motion остаётся статическим рядом с текстом «Загрузка». Это отличается от точных значений DS-08/12/18/19/20/25; внутренние селекторы UIKit не переопределяются.

## Контраст

Проверка выполнена в Chromium 157.0.8103.78 24 ноября 2026 года с итоговыми стилями и Provider каждой из четырёх тем. Использованы вычисленные браузером sRGB-цвета. Прозрачная поверхность накладывается на base-background, затем текст на полученную поверхность. Контраст вычисляется как `(Lmax + 0.05) / (Lmin + 0.05)` после линеаризации sRGB.

Проверены 70 пар на тему, включая текст подсказок, hover и active границ полей, Checkbox/Radio, а также текст и границы popup. Ниже префикс `--g-color-` опущен. Порог обычного текста 4,5:1, границ и фокуса 3:1. Значения округлены до трёх знаков только для таблицы.

| Передний план               | Поверхность           | Порог | Light  | Dark   | Light HC | Dark HC |
| --------------------------- | --------------------- | ----- | ------ | ------ | -------- | ------- |
| `text-primary`              | `base-background`     | 4.5:1 | 15.085 | 12.241 | 21.000   | 18.840  |
| `text-secondary`            | `base-background`     | 4.5:1 | 7.557  | 11.250 | 7.557    | 12.786  |
| `text-hint`                 | `base-background`     | 4.5:1 | 7.557  | 7.921  | 7.557    | 9.003   |
| `text-link`                 | `base-background`     | 4.5:1 | 9.187  | 8.979  | 9.187    | 10.205  |
| `text-link-hover`           | `base-background`     | 4.5:1 | 11.506 | 10.259 | 11.506   | 11.660  |
| `text-link-visited`         | `base-background`     | 4.5:1 | 8.720  | 12.177 | 8.720    | 13.840  |
| `text-link-visited-hover`   | `base-background`     | 4.5:1 | 10.876 | 14.049 | 10.876   | 15.967  |
| `line-generic`              | `base-background`     | 3:1   | 4.501  | 5.768  | 4.501    | 6.556   |
| `line-generic-solid`        | `base-background`     | 3:1   | 4.501  | 5.768  | 4.501    | 6.556   |
| `line-generic-accent`       | `base-background`     | 3:1   | 7.054  | 8.480  | 7.054    | 9.638   |
| `line-generic-hover`        | `base-background`     | 3:1   | 7.054  | 8.480  | 7.054    | 9.638   |
| `line-generic-active`       | `base-background`     | 3:1   | 5.699  | 8.979  | 5.699    | 10.205  |
| `line-generic-accent-hover` | `base-background`     | 3:1   | 10.437 | 11.250 | 10.437   | 12.786  |
| `line-brand`                | `base-background`     | 3:1   | 5.699  | 8.979  | 5.699    | 10.205  |
| `line-danger`               | `base-background`     | 3:1   | 8.310  | 8.766  | 8.310    | 9.964   |
| `line-focus`                | `base-background`     | 3:1   | 5.699  | 8.979  | 5.699    | 10.205  |
| `text-primary`              | `base-generic`        | 4.5:1 | 13.838 | 9.362  | 14.841   | 14.537  |
| `text-secondary`            | `base-generic`        | 4.5:1 | 6.766  | 8.334  | 5.341    | 9.865   |
| `text-hint`                 | `base-generic`        | 4.5:1 | 6.766  | 5.868  | 5.341    | 6.946   |
| `text-link`                 | `base-generic`        | 4.5:1 | 8.225  | 6.652  | 6.493    | 7.874   |
| `text-link-hover`           | `base-generic`        | 4.5:1 | 10.301 | 7.600  | 8.132    | 8.996   |
| `text-link-visited`         | `base-generic`        | 4.5:1 | 7.807  | 9.021  | 6.163    | 10.679  |
| `text-link-visited-hover`   | `base-generic`        | 4.5:1 | 9.736  | 10.407 | 7.686    | 12.320  |
| `line-generic`              | `base-generic`        | 3:1   | 4.029  | 4.273  | 3.181    | 5.059   |
| `line-generic-solid`        | `base-generic`        | 3:1   | 4.029  | 4.273  | 3.181    | 5.059   |
| `line-generic-accent`       | `base-generic`        | 3:1   | 6.315  | 6.282  | 4.985    | 7.437   |
| `line-generic-hover`        | `base-generic`        | 3:1   | 6.315  | 6.282  | 4.985    | 7.437   |
| `line-generic-active`       | `base-generic`        | 3:1   | 5.102  | 6.652  | 4.027    | 7.874   |
| `line-generic-accent-hover` | `base-generic`        | 3:1   | 9.343  | 8.334  | 7.376    | 9.865   |
| `line-brand`                | `base-generic`        | 3:1   | 5.102  | 6.652  | 4.027    | 7.874   |
| `line-danger`               | `base-generic`        | 3:1   | 7.440  | 6.494  | 5.873    | 7.688   |
| `line-focus`                | `base-generic`        | 3:1   | 5.102  | 6.652  | 4.027    | 7.874   |
| `text-primary`              | `base-float`          | 4.5:1 | 15.085 | 9.331  | 21.000   | 14.492  |
| `text-secondary`            | `base-float`          | 4.5:1 | 7.557  | 8.303  | 7.557    | 9.835   |
| `text-hint`                 | `base-float`          | 4.5:1 | 7.557  | 5.846  | 7.557    | 6.925   |
| `text-link`                 | `base-float`          | 4.5:1 | 9.187  | 6.627  | 9.187    | 7.850   |
| `text-link-hover`           | `base-float`          | 4.5:1 | 11.506 | 7.572  | 11.506   | 8.969   |
| `text-link-visited`         | `base-float`          | 4.5:1 | 8.720  | 8.987  | 8.720    | 10.646  |
| `text-link-visited-hover`   | `base-float`          | 4.5:1 | 10.876 | 10.369 | 10.876   | 12.282  |
| `line-generic`              | `base-float`          | 3:1   | 4.501  | 4.257  | 4.501    | 5.043   |
| `line-generic-solid`        | `base-float`          | 3:1   | 4.501  | 4.257  | 4.501    | 5.043   |
| `line-generic-accent`       | `base-float`          | 3:1   | 7.054  | 6.259  | 7.054    | 7.414   |
| `line-generic-hover`        | `base-float`          | 3:1   | 7.054  | 6.259  | 7.054    | 7.414   |
| `line-generic-active`       | `base-float`          | 3:1   | 5.699  | 6.627  | 5.699    | 7.850   |
| `line-generic-accent-hover` | `base-float`          | 3:1   | 10.437 | 8.303  | 10.437   | 9.835   |
| `line-brand`                | `base-float`          | 3:1   | 5.699  | 6.627  | 5.699    | 7.850   |
| `line-danger`               | `base-float`          | 3:1   | 8.310  | 6.470  | 8.310    | 7.664   |
| `line-focus`                | `base-float`          | 3:1   | 5.699  | 6.627  | 5.699    | 7.850   |
| `text-brand-contrast`       | `base-brand`          | 4.5:1 | 5.699  | 9.318  | 5.699    | 9.318   |
| `text-brand-contrast`       | `base-brand-hover`    | 4.5:1 | 6.408  | 10.645 | 6.408    | 10.645  |
| `text-positive`             | `base-positive-light` | 4.5:1 | 8.549  | 9.806  | 8.549    | 9.806   |
| `text-positive-heavy`       | `base-positive-light` | 4.5:1 | 8.549  | 9.806  | 8.549    | 9.806   |
| `text-primary`              | `base-positive-light` | 4.5:1 | 14.358 | 10.354 | 19.704   | 13.768  |
| `text-secondary`            | `base-positive-light` | 4.5:1 | 7.091  | 9.344  | 7.091    | 9.344   |
| `text-positive`             | `base-background`     | 4.5:1 | 9.111  | 11.806 | 9.111    | 13.418  |
| `text-warning`              | `base-warning-light`  | 4.5:1 | 7.892  | 10.868 | 7.892    | 10.868  |
| `text-warning-heavy`        | `base-warning-light`  | 4.5:1 | 7.892  | 10.868 | 7.892    | 10.868  |
| `text-primary`              | `base-warning-light`  | 4.5:1 | 14.021 | 10.204 | 19.113   | 13.535  |
| `text-secondary`            | `base-warning-light`  | 4.5:1 | 6.878  | 9.185  | 6.878    | 9.185   |
| `text-warning`              | `base-background`     | 4.5:1 | 8.671  | 13.310 | 8.671    | 15.128  |
| `text-danger`               | `base-danger-light`   | 4.5:1 | 7.513  | 7.855  | 7.513    | 7.855   |
| `text-danger-heavy`         | `base-danger-light`   | 4.5:1 | 7.513  | 7.855  | 7.513    | 7.855   |
| `text-primary`              | `base-danger-light`   | 4.5:1 | 13.946 | 11.056 | 18.986   | 14.853  |
| `text-secondary`            | `base-danger-light`   | 4.5:1 | 6.833  | 10.080 | 6.833    | 10.080  |
| `text-danger`               | `base-background`     | 4.5:1 | 8.310  | 8.766  | 8.310    | 9.964   |
| `text-info`                 | `base-info-light`     | 4.5:1 | 7.836  | 7.631  | 7.836    | 7.631   |
| `text-info-heavy`           | `base-info-light`     | 4.5:1 | 7.836  | 7.631  | 7.836    | 7.631   |
| `text-primary`              | `base-info-light`     | 4.5:1 | 13.876 | 10.348 | 18.865   | 13.760  |
| `text-secondary`            | `base-info-light`     | 4.5:1 | 6.789  | 9.339  | 6.789    | 9.339   |
| `text-info`                 | `base-background`     | 4.5:1 | 8.722  | 9.192  | 8.722    | 10.448  |

## Проверка

`npm test -- src/shared/ui/states.test.tsx` проверяет подписи и роли встроенных состояний, четыре причины пустоты, безопасные поля ошибки и повтор, отдельную дату снимка, версию успеха, копирование с fallback, скрытые значения и штатные сроки уведомлений.

В браузере проверены все четыре темы при 1280×800, 1920×1080, 768×1024 и 320×800. Для масштаба 200% и 400% на экране 1280×800 эмулированы соответствующие CSS-области 640×400 с DPR=2 и 320×200 с DPR=4. Подписи, действия и длинные значения остаются доступны, горизонтальной прокрутки страницы нет. Высота LoadingState измерена для вариантов 160 и 320 px.

Проверены hover, focus и invalid полей, read-only и disabled, список Select высотой до 320 px со строками от 44 px, клавиатурный вход в Tooltip и Escape, копирование полного SHA, совпадение глифов и цветов ошибки/успеха с тостами, reduced motion и различимость состояний без цвета. Временные страницы в продуктовые маршруты и Git не входят. Дополнительно просмотрена статическая отрисовка светлой и тёмной темы в Firefox; его интерактивные сценарии отдельно не прогонялись. Реальный API этим прогоном не проверяется.
