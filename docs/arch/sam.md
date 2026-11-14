# SAM: импорт инвентаризации и каталог

Задача 18 (импорт) и 17 (каталог). Схема БД — блок 15, файл
`server/migrations/00005_create_sam.sql`; этот документ описывает только
schema-agnostic часть, которая не зависит от таблиц.

## Что уже есть

- `server/internal/sam/importer` — разбор CSV, нормализация, фильтр обновлений,
  дедупликация и отчёт. Пакет не открывает БД.
- `server/internal/sam/catalog` — нормализация `demo/sam/catalog.json` в
  детерминированный seed вендоров, продуктов, алиасов и лицензий.
- `server/cmd/sam-seed` — подготовка seed из каталога:

```bash
cd server
go run ./cmd/sam-seed -in ../demo/sam/catalog.json -out seed/sam/catalog.json
```

Без `-out` seed печатается в stdout и не меняет репозиторий.

## Инвентаризация: маппинг CSV → `importer.Row`

Разделитель определяется автоматически (`,` `;` таб `|`), поддерживается
строка `sep=;`, UTF-8 BOM и кавычки CSV. Заголовки сравниваются без регистра,
пунктуации и с транслитерацией кириллицы.

| Поле `Row` | Основные заголовки |
|---|---|
| `Device` | `Device`, `Device Name`, `Computer Name`, `Hostname`, `Machine`, `Имя устройства` |
| `Name` | `Name`, `Software Name`, `Product Name`, `Display Name`, `Application`, `Наименование ПО` |
| `Publisher` | `Publisher`, `Vendor`, `Manufacturer`, `Software Publisher`, `Издатель` |
| `Version` | `Version`, `Product Version`, `Display Version`, `Версия` |
| `Architecture` | `Architecture`, `Arch`, `OS Architecture`, `Bitness`, `Архитектура` |
| `Edition` | `Edition`, `Release`, `SKU`, `Редакция` |
| `Update` | `Update`, `Update ID`, `KB`, `KB ID`, `Hotfix`, `Patch`, `Обновление` |
| `InstallDate` | `Install Date`, `Installed On`, `Installation Date`, `Дата установки` |
| `Source` | `Source`, `Source File`, `Inventory Source`, `Источник` |

Обязательна только колонка с названием ПО. Нераспознанные колонки игнорируются;
несовпадение числа колонок и строки без названия попадают в `Report.Issues`.

## Нормализация

- регистр и кириллица → латиница (`Transliterate`);
- пунктуация, пробелы, `™`, `®`, `©` убираются; `&`, `+`, `#` сохраняются словами
  (`and`, `plus`, `sharp`);
- версия: `v`/`Version`, разделители, ведущие и хвостовые нули, build-метаданные
  и архитектура в скобках (`v1.02.0.0` → `1.2`);
- архитектура: `amd64`, `386`, `arm64`, `arm`, `any`;
- вендор: юридические суффиксы (`Inc`, `LLC`, `GmbH`, `Corp`, …) и артикль `The`;
- устройство: FQDN сокращается до короткого имени, разделители удаляются
  (`PC-001.corp.local` → `pc001`);
- `IsUpdate` отсекает KB, `Update for`, `Security/Cumulative/Feature Update`,
  `Servicing Stack`, `Hotfix`, `Service Pack`, `Patch for`. Продукты вроде
  `VMware Update Manager` и `Java 8 Update 411` не отсекаются.

`DedupeKey` = нормализованные `Device + Name + Version` через `\x1f`.
Идемпотентный upsert позже использует тот же ключ, поэтому повторный импорт не
плодит записи.

## Отчёт импорта

`Report` соблюдает инвариант:

```
Total = Accepted + Duplicates + Empty + Updates + Errors
```

- `Updates` — обновления и KB, не являющиеся отслеживаемым ПО;
- `Empty` — строки без publisher или version (`EmptyPublisher`, `EmptyVersion`
  уточняют, чего не хватает);
- `Duplicates` — строки с тем же `DedupeKey`, что уже принятая;
- `Errors` — пустое имя, битая строка CSV, неверное число колонок, не-UTF-8.

Коды ошибок стабильны: `csv.parse`, `csv.read`, `row.empty`, `row.columns`,
`row.encoding`, `row.empty_name`, `row.empty_publisher_or_version`.

## Каталог: `demo/sam/catalog.json`

Поддерживается массив продуктов или объект с секциями:

```json
{
  "vendors": ["Microsoft"],
  "products": [
    {
      "vendor": "Microsoft Corporation",
      "name": "Windows 11 Pro",
      "aliases": ["Windows 11 Professional"],
      "category": "Operating system",
      "licenses": [
        {"metric": "per_device", "quantity": 50, "expiresAt": "2027-12-31"}
      ]
    }
  ],
  "licenses": [
    {"product": "Google Chrome", "vendor": "Google LLC", "metric": "per_device", "quantity": 120}
  ]
}
```

| Поле seed | Синонимы в JSON |
|---|---|
| `name` | `name`, `product`, `productName`, `title`, `software`, `displayName` |
| `vendor` | `vendor`, `publisher`, `manufacturer`, `company` |
| `aliases` | `aliases`, `alias`, `aka`, `synonyms` (массив или строка через запятую) |
| `category` | `category`, `type`, `kind` |
| `metric` | `metric`, `unit`, `licenseMetric`, `licenseType` |
| `quantity` | `quantity`, `seats`, `count`, `qty`, `amount` |
| `expiresAt` | `expiresAt`, `expires`, `expiry`, `validUntil`, `validUntilDate` |
| `notes` | `notes`, `note`, `description`, `comment` |

Продукты дедуплицируются по `vendor + name`, алиасы — по нормализованному
названию. Если лицензия ссылается на отсутствующий продукт, запись не попадает
в seed, а в `LoadResult.Issues` появляется код `catalog.license_unknown_product`.

## Что заблокировано

CRUD каталога, поиск/фильтры/пагинация, мягкая архивация и
`POST /api/agent/inventory` ждут `00005_create_sam.sql` от блока 15. До появления
схемы нельзя выбирать имена таблиц, колонок и sqlc-запросы, не создавая
временные таблицы и моки. Исходный `demo/sam/catalog.json` — результат блока
каталога; до его появления проверка идёт на маленьком фикстуре
`server/internal/sam/catalog/testdata/catalog.json`.

## Проверки

```bash
cd server
go test ./internal/sam/... -count=1
go vet ./internal/sam/...
go build ./cmd/...
```

Тест `TestImportCSV24500RowsUnder30Seconds` проверяет критерий объёма на
синтетическом наборе, а `TestAnalyzeProductionScaleCounts` — отсечение 8 650
обновлений, 420 пустых publisher/version и дубли по device+name+version.

## SAM: AI-ассистент и AI-оператор

Домен SAM хранит продукты, установки, алиасы и лицензии. Поверх него работают два
режима с разными правами:

| Режим | Доступ | Что делает | Модель |
|---|---|---|---|
| Ассистент-аналитик | любой авторизованный участник | читает данные через 7 read-only инструментов | `llm.Complete` с tool-loop до 4 раундов |
| AI-оператор | предложения — участник, apply/undo — администратор | готовит preview изменений, применяет после подтверждения | `llm.Complete` с 6 `propose_*` инструментами |

Модель никогда не получает SQL, не выполняет DELETE/DDL и не меняет данные сама.
Все операции идут через sqlc-запросы и сервисный слой.

## Схема

Миграции: `server/migrations/00005_create_sam_core.sql` (ядро SAM и диалоги),
`00006_create_sam_ai_actions.sql` (журнал предложений оператора).

| Таблица / view | Назначение |
|---|---|
| `sam_products` | продукты, вендор, версия, категория, статус `active/inactive/archived` |
| `sam_installations` | обезличенные установки; `product_id = NULL` означает «не сопоставлено» |
| `sam_product_aliases` | нормализованные алиасы для поиска и сопоставления |
| `sam_licenses` | тип, места, стоимость места, даты, статус лицензии |
| `sam_risk_overview` | view с рисками: несопоставленные установки, истёкшие лицензии, перерасход мест, неактивные продукты, продукты без лицензии |
| `sam_ai_conversations` | диалоги ассистента и оператора; `mode = assistant/operator` |
| `sam_ai_messages` | сообщения диалога; tool-сообщения модели не публикуются в API |
| `sam_ai_actions` | предложение оператора: payload, preview, status, affected, before/after, fingerprint, audit-поля |

`sam_ai_actions.status`: `proposed → confirmed → applied`, либо `cancelled/failed`.
`confirmed` живёт только внутри транзакции apply; после коммита остаётся `applied`.
Повторный apply возвращает уже применённое действие и не пишет второй раз.

## Ассистент

Пакет: `server/internal/sam/assistant`.

Эндпоинты:

| Метод | Путь | Ответ |
|---|---|---|
| POST | `/api/sam/assistant/chat` | `{id, answer, sources, fallback}` |
| GET | `/api/sam/assistant/conversations` | список своих диалогов |
| GET | `/api/sam/assistant/conversations/{id}` | сообщения своего диалога |

Инструменты (каждый — отдельный sqlc-запрос):

- `search_products` — `sam_products` + `sam_product_aliases`;
- `list_products(status)` — `sam_products`;
- `product_balance` — оплаченные места, установки, свободные места, ближайшее истечение;
- `find_installations` — обезличенные установки, без имён сотрудников и устройств;
- `license_summary` — сводка по типу лицензии;
- `top_risks` — `sam_risk_overview`;
- `unmatched_names` — несопоставленные имена и количество устройств.

Системный промт запрещает выдумывать данные, требует цифры и источник
(`sam_licenses, license_summary` и т. п.) и прямо запрещает изменения.

Tool-loop ограничен 4 раундами и 12 вызовами. Если модель недоступна, вернула
ошибку, не вызвала ни один инструмент или не дала финальный текст, включается
детерминированный интент-роутер. Он отвечает без модели на базовые вопросы:
сводка, установки, баланс продукта, лицензии, риски, несопоставленные имена,
поиск и список продуктов. Это гарантирует числа даже при недоступном LLM.

Лимит запросов общий с основным чатом: один запрос на пользователя и два
одновременно на процесс (`sam/guard.Limiter`), при перегрузке — `429 chat.busy`.

## AI-оператор

Пакет: `server/internal/sam/operator`.

| Метод | Путь | Права |
|---|---|---|
| POST | `/api/sam/operator/chat` | авторизованный участник; создаёт только `proposed` |
| POST | `/api/sam/operator/actions/{id}/apply` | администратор (`identity.RequireAdmin`) |
| POST | `/api/sam/operator/actions/{id}/cancel` | автор предложения или администратор |
| POST | `/api/sam/operator/actions/{id}/undo` | администратор, только последнее `applied` |
| GET | `/api/sam/operator/actions` | свои действия; администратор видит все |

Инструменты модели: `propose_match_aliases`, `propose_set_status`,
`propose_set_category`, `propose_update_license`, `propose_archive_product`,
`propose_create_product`. Других инструментов нет.

### Preview

`buildPlan` пересчитывает цель по текущей БД и формирует:

- `affected` — сколько строк будет изменено;
- `samples` — первые 20 примеров;
- `diffs` — old → new;
- `warnings` — конфликты и ограничения;
- `requiresPhrase` и `confirmationPhrase` — для запроса «изменить всё»;
- `fingerprint` — SHA-256 от состава строк и старых значений.

Создание `sam_ai_actions` и сообщений диалога — единственная запись на этапе
preview. Продукты, установки, алиасы и лицензии не меняются, это проверяют
unit- и database-тесты.

### Apply

1. Проверяется администратор и контрольная фраза (если требуется).
2. В транзакции действие блокируется `FOR UPDATE`, статус переходит в `confirmed`.
3. Plan пересчитывается заново; при другом `affected` или `fingerprint` транзакция
   откатывается и возвращается `409 operator.stale_preview`.
4. sqlc-обновления проверяют `RowsAffected`; несовпадение — тоже stale.
5. Сохраняются `before/after`, `applied_at`, пишется `audit_events`.
6. Повторный apply видит `applied` и возвращает действие без повторной записи.

### Ограничения и откат

- Максимум 5000 изменяемых строк на действие (`operator.limit_exceeded`).
- `all=true` требует контрольную фразу `ПОДТВЕРЖДАЮ ИЗМЕНЕНИЕ ВСЕХ`.
- Undo разрешён только для последнего `applied` действия, чтобы журнал оставался
  линейным. Восстановление идёт из `before`/`after`: статусы, категории, лицензии,
  алиасы и `sam_installations.product_id`. Созданный продукт удаляется только если
  от него уже нет зависимостей, иначе `409 operator.undo_conflict`.

### Сценарий «сопоставь все хромы с Google Chrome»

1. Модель вызывает `propose_match_aliases` с продуктом `Google Chrome` и `all=true`
   (или интент-роутер делает это при недоступной модели).
2. Сервис ищет несопоставленные raw-имена по стемам продукта (`goog`, `chro`,
   `chrom`), проверяет конфликты алиасов и строит preview.
3. Preview показывает установки, будущие алиасы, diff и контрольную фразу.
4. После apply в одной транзакции создаются `sam_product_aliases` и обновляется
   `sam_installations.product_id`; количество вставок и обновлений сверяется с preview.
5. Undo удаляет созданные алиасы и возвращает установки в `product_id = NULL`.

## Проверки

```bash
# сервер целиком (нужен TEST_DATABASE_URL на portal_test)
cd server && go test -p 1 ./... && go vet ./... && go build ./cmd/...

# только SAM
go test -p 1 ./internal/sam/...

# контракт
bun run gen:api
```

Тесты с фейковой моделью проверяют tool-loop, fallback, preview без записи,
идемпотентный apply, stale 409, лимит, RBAC, audit и undo. Database-тесты
дополнительно прогоняют реальный PostgreSQL: 10 вопросов ассистента с точными
числами и полный жизненный цикл оператора на sqlc-запросах.

## Допущения

- Блок 15 (базовая схема SAM) в живой ветке `task/2-public-planner` отсутствовал,
  поэтому `00005_create_sam_core.sql` добавлен как обязательный предшественник.
- Ассистент доступен любому авторизованному участнику, а не анонимному интернету:
  данные обезличены, но диалоги и лимиты привязаны к пользователю.
- Стоимость лицензии хранится в целых рублях за место за год.
