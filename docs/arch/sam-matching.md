# SAM: Smart Matching (блок 19)

Smart Matching превращает «грязные» имена из инвентаризации в кандидатов каталога.
Он не меняет установки и алиасы сам: сначала создаётся очередь `pending`, затем
человек подтверждает или отклоняет кандидата.

## Данные

`server/migrations/00007_create_sam_matching.sql`:

| Объект | Назначение |
|---|---|
| `sam_product_aliases.confidence` | уверенность алиаса `0..1`; ручное подтверждение ставит `1` |
| `sam_match_candidates` | очередь кандидатов: установка, продукт, score, method, status |
| `sam_match_candidates.status` | `pending → confirmed/rejected` |

Один кандидат уникален по паре `installation_id + product_id`. Повторный запуск
обновляет score у `pending` и не трогает уже решённые строки.

## Нормализация и score

`server/internal/sam/matching/normalize.go`:

- приводит к нижнему регистру, заменяет `ё`, убирает пунктуацию;
- удаляет `exe`, `msi`, `setup`, `install`, `x64`, версии, `KB`, годы;
- переводит русские названия вендоров: `хром → chrome`, `гугл → google`,
  `майкрософт → microsoft`, `фотошоп → photoshop`;
- считает score по трём правилам:
  - точное совпадение нормализованных строк — `1.0`, `exact`;
  - вхождение или Jaccard по токенам — `token`;
  - общий префикс от 5 символов и Levenshtein — `fuzzy`.

Порог — `0.7`. Для одной установки сохраняются до трёх лучших продуктов, чтобы
человек мог выбрать правильный вариант в очереди.

Примеры, которые покрыты тестами:

| Грязное имя | Кандидат |
|---|---|
| `chrome.exe` | Google Chrome |
| `Google_Chrome_v2` | Google Chrome |
| `chrome_setup` | Google Chrome |
| `Хром экзе` | Google Chrome |
| `Google Chrome (x64)` | Google Chrome |
| `chromium` | Google Chrome |
| `Фотошоп 2024` | Adobe Photoshop (по алиасу) |
| `unknown-zzz` | нет кандидата |

## API

Все операции требуют сессию участника.

| Метод | Путь | Что делает |
|---|---|---|
| POST | `/api/sam/matching/run` | строит очередь кандидатов, не меняя установки и алиасы |
| GET | `/api/sam/matching/candidates?status=pending` | очередь с продуктом, устройством и score |
| GET | `/api/sam/matching/summary` | `unmatched`, `pending`, `confirmed`, `rejected` |
| POST | `/api/sam/matching/candidates/{id}/confirm` | создаёт алиас, обновляет `product_id`, аудит |
| POST | `/api/sam/matching/candidates/{id}/reject` | отклоняет кандидата, аудит |

`run` ограничен 5 000 установок за вызов. В ответе: `scanned`, `unmatched`,
`matched`, `candidates`, `skipped`, `pending`.

При подтверждении в одной транзакции:

1. кандидат блокируется `FOR UPDATE` и должен быть `pending`;
2. проверяется, что нормализованный алиас не принадлежит другому продукту;
3. создаётся/усиливается алиас с `confidence = 1`;
4. установка получает `product_id`, если он ещё пуст;
5. остальные `pending`-кандидаты этой установки отклоняются;
6. пишется `audit_events` (`sam_matching.confirm/reject`).

Повторное подтверждение или изменение уже решённого кандидата возвращает `409`.

## Проверки

```bash
go test -p 1 ./internal/sam/matching
```

Unit-тесты проверяют нормализацию, score, отсутствие записи при `run`, confirm,
reject, конфликт алиаса и повторное подтверждение. Database-тест прогоняет
реальный PostgreSQL: 5 «хромов» + неизвестное имя, confirm/reject, счётчики и аудит.

UI очереди — блок 23 (frontend). API уже отдаёт всё необходимое для экрана.
