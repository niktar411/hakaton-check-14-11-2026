# SAM: комплаенс и риски (блок 20)

Комплаенс-движок считает баланс «куплено против установлено», финансовые потери,
юридические риски и рекомендации. Только чтение.

## Схема

`server/migrations/00008_create_sam_compliance.sql`:

- добавляет статус `banned` для `sam_products`;
- обновляет `sam_risk_overview`: запрещённые продукты теперь отдельный
  high-риск, а `inactive_products_installed` считает только
  `inactive/archived`.

Лицензия на устройство: установки считаются по уникальным
`sam_installations.device_name`; пустое имя заменяется на стабильный
`installation:<id>`. Несколько версий одного продукта на одном устройстве дают
одну занятую лицензию. Просроченная лицензия не покрывает установки.

## Формулы

Для каждого продукта:

- `licensedSeats` — сумма мест только по действующим лицензиям
  (`active` и `expires_at IS NULL OR >= current_date`);
- `expiredSeats` — места активных лицензий с истёкшим сроком;
- `installedDevices` — уникальные устройства с установленным продуктом;
- `overusedSeats = max(installedDevices - licensedSeats, 0)`;
- `idleSeats = max(licensedSeats - installedDevices, 0)`;
- `avgCostPerSeat` — средняя цена места по всем активным лицензиям;
- `annualCost` — стоимость действующих мест;
- `riskCost = (overusedSeats + expiredSeats + idleSeats) * avgCostPerSeat`.

Запрещённые продукты исключены из расчёта overuse/idle/expired, чтобы не
задваивать риск: для них считается `bannedInstallations`.

Итог:

- `costAtRisk = overuseCost + expiredCost`;
- `idleCost` показывается отдельно как замороженные деньги;
- `riskLevel`: `high`, если есть overuse, expired или banned-установки;
  `medium`, если есть idle или несопоставленные; иначе `low`.

## API

| Метод | Путь | Ответ |
|---|---|---|
| GET | `/api/sam/compliance/summary` | KPI, сумма риска, рекомендации |
| GET | `/api/sam/compliance/products` | баланс и риск по каждому продукту |
| GET | `/api/sam/compliance/risks` | отсортированный список рисков |

Все три требуют сессию участника.

`summary` содержит: продукты, активные/запрещённые, установки, устройства,
несопоставленные, места, overuse/idle/expired, годовую стоимость, стоимости
рисков, `costAtRisk`, `riskLevel`, массив рекомендаций.

`products` содержит по продукту: лицензии, установки, overuse/idle, стоимость,
`riskCost`, `riskLevel` и текстовую рекомендацию.

`risks` — плоский список: `code`, `severity`, `productId/productName`,
`affected`, `cost`, `recommendation`. Коды: `banned_product`, `license_overuse`,
`expired_license`, `idle_license`, `inactive_product`,
`unmatched_installations`.

## Рекомендации

- запрещённое ПО → удалить с N устройств;
- превышение → докупить N мест или удалить лишние установки;
- просрочка → продлить или заменить N мест;
- простой → вернуть или перераспределить N мест;
- несопоставленные → сопоставить N установок с каталогом.

## Проверки

```bash
go test -p 1 ./internal/sam/compliance
```

Unit-тесты проверяют формулы, уровни риска и рекомендации. Database-тест на
реальном PostgreSQL собирает сценарий: Office 118/100, Project 10/50,
Photoshop 2 без лицензии, Radmin banned 8, Slack 5/5, Java с просроченной
лицензией и 3 несопоставленные установки. Сверяются KPI, стоимости, продукты,
риски и рекомендации.

UI дашборда — блок 22 (frontend). API уже отдаёт данные для KPI и таблицы рисков.
