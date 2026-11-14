# SAM: реальные обезличенные данные и демо-пул

> **DEMO.** Исходная инвентаризация очищена: серийники заменены на
> `PC-0001…PC-0187`, сотрудники и хосты удалены. Купленные места, цены и
> `last_used_at` синтетические и помечены `demo: true`.

## Источники

| Файл | Где лежит | Коммитить |
|---|---|---|
| `INSTALLED_SOFTWARE.xlsx` — инвентаризация | `demo/sam/source/` (gitignored) | нет |
| `SOFTWARE.xlsx` — пример перечня ПО | `demo/sam/source/` (gitignored) | нет |
| `LICENSES.xlsx` — перечень лицензий | `demo/sam/source/` (gitignored) | нет |
| Соль и mapping | `~/.config/sam-demo/` (0600) | нет |

Исходные материалы организаторов находятся в ветке `task/case-materials-sam`.
В эту ветку попадают только производные обезличенные файлы.

## Производные файлы

| Файл | Содержимое |
|---|---|
| `installed_software.csv` | 24 589 строк, 187 ПК, флаги `is_trackable`, `is_duplicate`, `is_demo` |
| `catalog.json` | каталог в формате `server/internal/sam/catalog.Load` |
| `licenses.json` | синтетические купленные места (Office, Project) |
| `usage.json` | синтетический `last_used_at` по парам устройство+продукт |
| `summary.json` | счётчики и самопроверка |

## Фактические счётчики

| Показатель | Значение |
|---|---:|
| Строк в выгрузке | **24 589** |
| Устройств `PC-0001…PC-0187` | **187** |
| Обновления/KB/Hotfix (`is_trackable=false`) | **8 590** |
| Учитываемое ПО (`is_trackable=true`) | 15 999 |
| Пустой `publisher` | 0 |
| Пустой `version` | **68** |
| Дубли по `device_id + product_name + version` | 21 |
| Уникальных названий | 4 105 |
| Издателей | 306 |

План кейса оперирует числом **24 521** — это строки с непустой версией:
`24 589 − 68 = 24 521`. Остальные цифры плана (8 650 обновлений, 420 пустых,
27 дублей) из этой выгрузки не воспроизводятся: фактические значения выше.

## Демо-кейсы

| Кейс | Факт |
|---|---|
| Microsoft Office | 119 ПК, 100 мест, разрыв 19 |
| Microsoft Project | 68 ПК, 50 мест, 10 активных |
| Adobe Photoshop | 2 ПК, лицензии нет |
| Radmin | 8 ПК, статус `banned` |

## Формат CSV

```
row_id,device,product_id,product_name,publisher,version,install_date,is_trackable,is_duplicate,is_demo
```

- `device` — только `PC-NNNN`; серийников и хостов нет. Имя колонки совместимо
  с `server/internal/sam/importer`.
- `product_id` — название продукта из `catalog.json`; пусто для несопоставленных.
- `is_trackable=false` — обновления/KB/хотфиксы.
- `is_duplicate=true` — повтор ключа.
- Пустые поля сохраняются.

## Перегенерация

```bash
# 1. Экспорт xlsx в промежуточные CSV (требует системный unzip)
bun run demo:sam:source

# 2. Обезличивание и сборка пула
bun run demo:sam
```

Соль и mapping создаются автоматически в `~/.config/sam-demo/` с правами `0600`.
При сохранённых соли и mapping результат воспроизводим.

## Интеграция с сервером

- Каталог: `demo/sam/catalog.json` читает `server/cmd/sam-seed` и
  `server/internal/sam/catalog.Load`.
- Установки: `demo/sam/installed_software.csv` принимает
  `server/internal/sam/importer.ParseCSV` (колонки `serial`, `name`,
  `publisher`, `version`).
- UI `/sam` и миграции SAM находятся в `main`; статический `/sam` здесь
  намеренно не дублируется.

## Ограничения

- Купленные места, цены и `last_used_at` — синтетика.
- Статус `banned` для Radmin задаётся в БД/операторе, каталог его только
  помечает.
- Исходные xlsx и промежуточные CSV не коммитятся.
