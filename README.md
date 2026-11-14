# Проект хакатона

SolidJS + TypeScript (Vite, Tailwind v4, CSS Modules) · Go (chi + huma, sqlc + pgx, goose, slog) ·
PostgreSQL 18 · nginx (HTTP/3, Brotli, SSI) · Gitea + Gitea Actions.

Продукт собирается вокруг двух кейсов:

- **AI-платформа команды** — чат с моделью, командный планер, админка и
  асинхронный coding-агент. Описание кейса: `docs/cases/ai-chat.md`.
- **SAM — учёт и комплаенс ПО** — каталог разрешённого ПО, импорт инвентаризации,
  Smart Matching, комплаенс-дашборд, AI-ассистент и AI-оператор.
  Описание кейса: `docs/cases/sam-2026.md`.

## Быстрый старт

Предусловия: Bun 1.4+, Go 1.27+, Docker + Compose, PostgreSQL 18
(можно поднять из Compose).

```bash
bun install                       # зависимости JS
cp .env.example .env              # заполнить значения (см. таблицу ниже)
bun run build:client              # собрать статику в dist/
set -a && source .env && set +a   # переменные окружения
cd server && go run ./cmd/api     # API + миграции + статика на $PORTAL_LISTEN
```

Администратора создаёт отдельная команда (в другом терминале):

```bash
cd server
ADMIN_COLOR="#2563EB" go run ./cmd/bootstrap   # использует .env из шелла
```

PostgreSQL для локальной разработки:

```bash
# в .env добавить POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
MAIN_ENV_FILE=$PWD/.env docker compose up -d postgres
```

Полный стек (`postgres`, `api`, `web`, `orchestrator`) поднимается так же:

```bash
MAIN_ENV_FILE=$PWD/.env docker compose up -d --build postgres api web
```

`web` публикует 80/443 и требует TLS-сертификаты
(`TLS_CERTIFICATE_FILE`, `TLS_KEY_FILE`); для локальной проверки достаточно API
со статикой (`STATIC_DIR=dist`).

## Проверки

```bash
bun run check            # lint + slop + типы скриптов + тесты scripts/
bun run typecheck:client # типы клиента
bun run build:client     # сборка клиента
bun x playwright test    # браузерные e2e (нужен chromium)

cd server
go test -p 1 ./...       # тесты сервера (нужен TEST_DATABASE_URL)
go vet ./...
go build ./cmd/...
```

Контракт API генерируется, руками его не правим:

```bash
bun run gen:api          # api/openapi.json + client/src/shared/api/schema.d.ts
cd server && sqlc generate
```

Pre-push-хуки и CI повторяют те же проверки
(`.githooks/pre-push`, `.gitea/workflows/ci.yml`).

## Что где лежит

| Путь | Что это |
| --- | --- |
| `client/` | SolidJS-клиент: страницы, токены, shared/ui, Paraglide |
| `server/` | Go-сервер: huma-контракты, sqlc, миграции, SAM, чат, админка |
| `api/openapi.json` | Сгенерированный контракт API |
| `docs/arch/` | Архитектурные описания подсистем |
| `docs/cases/` | Описания кейсов и исходные материалы |
| `demo/sam/` | Обезличенные данные и демо-пул SAM |
| `deploy/`, `infra/` | Скрипты серверов, firewall, worker, nginx |
| `.gitea/workflows/` | CI (клиент, контракт, сервер) |

## Роли и доступ

| Роль | Что может | Как попадает в систему |
| --- | --- | --- |
| Аноним | публичный сайт `/`, планер (краткий вид), SAM read-only | без входа |
| Участник (`member`) | чат, полный планер, coding-задачи, деплойments | локальный вход на `/coding` |
| Администратор | админка, управление аккаунтами, apply/undo SAM-оператора | локальный вход с VPN-адреса |
| Хост-агент | `POST /api/agent/inventory` с токеном хоста | токен из `worker.env` |

Публичные SAM-эндпоинты не принимают записи. Оператор работает по схеме
`preview → confirm → apply → audit`, `apply`/`undo` — только администратор.

## Демо-сценарий

Сценарий на 5 минут с двумя пользователями описан в `docs/demo-script.md`.
Ключевые экраны: чат (`/chat`), планер (`/planner`), деплойments (`/deployments`),
SAM (`/sam`). Для двух аккаунтов откройте сайт в двух браузерах или окнах.

## Как здесь работать

- Правила для людей и нейросети — `AGENTS.md`; свод стека и дизайн-системы —
  `docs/playbook.md`.
- Новое — генераторами: `bun run new:page | new:component | new:route | new:migration`.
- Заглушки, моки, `any`, сырые цвета, произвольные классы Tailwind и русский
  текст в JSX не проходят: их ловят хуки, Biome, pre-push и CI
  (`scripts/slop-check.ts`).

## Окружение

Основные переменные (полный список и комментарии — `.env.example`):

| Переменная | Зачем |
| --- | --- |
| `PORTAL_LISTEN`, `STATIC_DIR` | адрес API и каталог статики |
| `PUBLIC_BASE_URL`, `PLANNER_BASE_URL`, `AI_BASE_URL` | публичные адреса сайтов |
| `DATABASE_URL`, `TEST_DATABASE_URL` | рабочая и тестовая базы |
| `MODEL_URL`, `MODEL_NAME`, `MODEL_API_KEY` | модель чата (DeepSeek V4.1 Flash) |
| `SETTINGS_ENCRYPTION_KEY` | шифрование ключа модели в админке (32 байта) |
| `ADMIN_USERNAME`, `ADMIN_VPN_IP` | администратор и разрешённые VPN-адреса |
| `GITLAB_URL`, `GIT_PROVIDER`, `GITLAB_*` | Gitea/GitLab: OAuth, токен, проект |
| `WORKER_URL`, `WORKER_TOKEN` | релей на coding-агент SERVER 2 |

Секреты в чат, карточки и коммиты не выносим: они живут в `.env`
или в файлах сервера (`/etc/hackathon/*.env`).

## Документация

| Файл | Что описывает |
| --- | --- |
| `AGENTS.md`, `CLAUDE.md` | правила работы команды и ИИ-агентов |
| `docs/playbook.md` | стек, дизайн-система, nginx, CI |
| `docs/cases/ai-chat.md` | кейс «AI-платформа команды»: роли и блоки |
| `docs/arch/ai-chat.md` | архитектура AI-чата: фазы, SSE, лимиты, контекст |
| `docs/demo-script.md` | сценарий демонстрации на 5 минут |
| `docs/team-workflow.md` | доска, статусы и ревью |
| `docs/arch/sam.md` | SAM: ассистент и оператор |
| `docs/arch/sam-matching.md`, `docs/arch/sam-compliance.md` | SAM: matching и комплаенс |
| `docs/arch/sam-side-parts.md` | SAM: согласие, дисклеймер, уведомления |

## Команда

Состав ниже читает единая публичная страница `/` при сборке клиента. Меняйте
только текст между маркерами `team:start` и `team:end` — этого достаточно,
чтобы обновить карточки участников на сайте.

<!-- team:start -->
**Название:** Драконы
**Девиз:** Создаём. Разрабатываем. Побеждаем.
**О команде:** «Драконы» — пять участников, одна команда и общая цель: делать качественные цифровые продукты и достойно представлять себя на чемпионатах по веб-разработке.

| Участник | Роль | Telegram | Цитата | Био | Фото |
| --- | --- | --- | --- | --- | --- |
| Никита Таргонский | Тимлид, fullstack, DevOps | @niktar411 | Умею всё — от контроллеров до сайтов, поэтому на хакатоне мне доверили ещё и кофе. | Студент СПК, не первый раз на хакатонах. Подрабатываю в разных IT-компаниях, многосторонний разработчик: работал с контроллерами, сайтами и различными системами. | /team/nikita.jpg |
| Егор Казанджи | Fullstack-разработчик | @Egorjian | Я не знаю как будет и я не знаю как быть. | Недо студент недо программист. | /team/egor.jpg |
| Игорь Василюк | Fullstack-разработчик, тестировщик | @hensy0 | Большие проекты начинаются с маленьких шагов и желания идти дальше. | Студент СПК, Full Stack-разработчик. Участник хакатонов с практическим опытом работы в IT-компаниях. Занимаюсь разработкой сайтов и веб-приложений, работаю с информационными системами. Постоянно развиваю навыки и осваиваю новые технологии. | /team/igor.jpg |
| Эльдар Вагапов | Дизайнер интерфейсов | @utka04 | Нормально делай — нормально будет. | Учусь в Школе 21 и хожу на все её хакатоны. Работаю дизайнером интерфейсов. | /team/eldar.jpg |
| Леонид Дворядкин | Fullstack-разработчик | @Kvantich | Лучше посрать и опоздать, чем прийти и обосраться. | Студент СПК, впервые на хакатоне. Занимаюсь самостоятельно, отдельно от колледжа: есть опыт в геймдеве, сейчас работаю с бэкендом. | /team/leonid.jpg |
<!-- team:end -->

## Командная работа

Общие инструкции для разных ИИ-клиентов: AGENTS.md.
Доска, статусы и ревью: docs/team-workflow.md. Шаблон: docs/task-template.md.
Проверка каркаса одной командой: bun run check.
