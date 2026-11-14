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
