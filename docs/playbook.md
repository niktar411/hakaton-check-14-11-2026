# Плейбук хакатона: дизайн, структура, правила, поддержка

Выжимка из ЛифтДжоба (что сработало, что болело) под новый стек:
**Go на бэке, SolidJS на фронте**. Файл — основа для `CLAUDE.md`, `docs/arch/*`
и первого коммита. Разделы 1–3 — решения, 4–6 — дизайн, 7–12 — инфраструктура
и ИИ-обвязка, 13 — чек-лист первого дня.

---

## 1. Стек: что остаётся, что меняем и почему

| Было (ЛифтДжоб / ТЗ) | На хакатоне | Почему |
|---|---|---|
| React 19 | **SolidJS** (последний стабильный; вышел 2.0 stable — брать 2.0) + `@solidjs/router` | Без VDOM, бандл в разы меньше, `renderToString` для пререндера есть из коробки |
| Vite | **Vite** + `vite-plugin-solid` | Плагины `ssiAfterAssets`/`inlineSmallCss` переезжают как есть |
| Hono + Bun (сервер) | **Go**, роутер **chi v5** + **huma v2** | huma даёт валидацию из struct-тегов **и OpenAPI 3.1 автоматически** — см. п. 2 |
| Bun (менеджер пакетов) | **Bun остаётся** для клиента и как оркестратор скриптов (`bun run dev` запускает и Go, и Vite) | Один вход для команд, хуки Claude пишем на Bun TS |
| Drizzle ORM | **sqlc + pgx/v5** | Drizzle только для TS. sqlc: пишешь SQL → получаешь типизированный Go. Ни рефлексии, ни магии, ИИ отлично пишет SQL |
| drizzle-kit | **goose** (миграции `.sql`, `-- +goose Up/Down`) | Те же правила: применённую миграцию не трогать |
| valibot (сервер) | **struct-теги huma** (`minLength`, `pattern`, `format:"email"`) | Схема одна — в Go; на клиент уходит через OpenAPI |
| valibot (клиент) | **valibot остаётся** + `@modular-forms/solid` | Формы Solid с valibot-адаптером |
| pino + pino-pretty | **log/slog** (stdlib, JSON в проде) + **lmittmann/tint** (цветной вывод в dev) | Стандарт Go, ноль зависимостей в проде |
| Paraglide | **Paraglide JS 2** (фреймворк-независимый) | Переезжает без изменений: компилируемые сообщения, tree-shaking |
| Атомарный CSS | **Tailwind CSS v4** (`@tailwindcss/vite`), тема в CSS через `@theme` | Нейросети знают его лучше всего → меньше выдуманных классов. Стандартная палитра **вырезана**: существуют только классы наших токенов (п. 5) |
| CSS Modules | **CSS Modules** — для компонентов | Как в ЛифтДжобе для нового кода |
| Biome | **Biome 2** (клиент; домен `solid`) + **golangci-lint v2** + **gofumpt** (Go) | Grit-плагины (no-italic, no-native-select) переносятся |
| bun test | **Vitest** + `@solidjs/testing-library` (клиент), **go test** + testcontainers-go (сервер) | Vitest нативно понимает трансформацию Solid |
| — | **air** (hot reload Go) | Аналог `bun --watch` |
| nginx + Brotli + HTTP/3 | **без изменений** | В ТЗ написано «HTML/3», имеется в виду **HTTP/3** (QUIC) |
| Postgres | **PostgreSQL 18** | — |

Версии: брать последние на момент старта и проверить командами
`bun outdated` и `go list -m -u all`. Lock-файлы (`bun.lock`, `go.sum`) коммитить.

**Где я не согласен с ТЗ.** «Всё асинхронное» в Go выглядит иначе: в Go нет
async/await, каждый HTTP-запрос и так идёт в своей горутине. Правило переводится так:
*никакой блокирующей работы в обработчике дольше ~100 мс* (тяжёлое уходит в
горутину/очередь с `context`), *у каждого I/O есть `ctx` с таймаутом*. На клиенте
правило остаётся буквальным: никаких синхронных XHR, всё через `createResource`/`query`.

---

## 2. Главное улучшение: один источник правды для API

Самая частая ошибка в ЛифтДжобе — **пары-зеркала сервер↔клиент** (валидаторы,
тарифы, лимиты, причины жалоб). Чтобы не забыть вторую сторону, понадобились
хук `mirrors.ts` и ревьюер-агент. На хакатоне этот класс ошибок убирается целиком:

```
Go-структуры с тегами  ──huma──▶  /openapi.json  ──openapi-typescript──▶  client/src/shared/api/schema.d.ts
                                                  ──скрипт gen-valibot──▶  client/src/shared/api/valibot.gen.ts
```

- Типы запросов и ответов, enum'ы (статусы, причины жалоб, тарифы) объявляются
  **только в Go**. Клиент импортирует сгенерированное.
- Клиент: `openapi-fetch` поверх `schema.d.ts`. Путь, метод, тело и ответ проверяются
  типами, голый `fetch` по-прежнему запрещён.
- Команда `bun run gen:api` запускает `go run ./cmd/openapi > api/openapi.json`
  и генерацию. CI запускает её и падает, если `git diff --exit-code` что-то находит
  (сгенерированное устарело).
- Константы, которых нет в API (лимиты медиа, размеры миниатюр), отдаются
  эндпоинтом `GET /api/config` или тоже кладутся в OpenAPI (`x-limits`).

---

## 3. Структура репозитория

ТЗ взято за основу, правки помечены ✱.

```
.claude/
  agents/            test-runner.md, reviewer.md, translator.md, ui-auditor.md ✱
  hooks/             guard.ts, slop-guard.ts ✱, file-size.ts ✱, lint.ts, gen-reminder.ts ✱,
                     prompt-router.ts ✱, done-gate.ts ✱, typecheck.ts, session-start.ts
  rules/             ✱ правила по путям: solid.md, go.md, sql.md, styles.md (п. 9.6)
  skills/            check, new-route, new-page, migration, i18n, seed ✱, prod-status
  settings.json
  launch.json        dev-серверы для превью в браузере Claude
.githooks/
  pre-push           lint → typecheck/vet → build → test (то же, что CI)
.github/             только README-бейджи / зеркало; CI живёт в GitLab ✱
api/
  openapi.json       ✱ сгенерированный контракт (коммитится, CI сверяет)
client/
  index.html         инлайн-скрипт темы, SSI-блок seo_head
  public/            llms.txt, robots.txt, manifest, icons, og-image
  scripts/           prerender.ts, i18n-sync.ts, gen-valibot.ts
  src/
    app/             main.tsx, App.tsx (маршруты), providers.tsx, prerender.tsx
    layout/          Layout, Header, Footer
    shared/
      api/           client.ts (openapi-fetch), schema.d.ts (gen)
      ui/            Button, Select, Checkbox, Dialog, Toast, Input, Card… (каждый + .module.css)
      lib/           consent.ts, money.ts, validation.ts, theme.ts
      styles/        tokens.css ✱, base.css, reset.css
    features/<домен>/  страницы и компоненты домена
    paraglide/       messages + locales/*.ts
  vite.config.ts
  nginx.conf, nginx.prod.conf
  snippets/security-headers.conf ✱
  Dockerfile         сборка + nginx с ngx_brotli и http3
deploy/
  bootstrap.sh       настройка сервера (docker, ufw, пользователь deploy)
  gitlab/            ✱ docker-compose GitLab CE + runner, init-скрипт аккаунтов
  init-tls.sh, backup.sh
docs/
  arch/              architecture, design-system ✱, frontend, backend, ci-cd, seo, dev-environment
  api.md, deploy.md
server/
  cmd/api/main.go    ✱ вход: конфиг, логгер, pgx-пул, роутер, graceful shutdown
  cmd/openapi/       ✱ печать OpenAPI без запуска сервера
  internal/
    config/          env → struct, проверка обязательных
    http/            router.go, middleware/ (auth, ratelimit, recover, requestid, logging)
    <домен>/         handler.go, service.go, types.go (huma-структуры)
    db/              sqlc-код (gen), pool.go, tx.go
    logger/
  migrations/        ✱ вместо server/drizzle/: goose 00001_init.sql …
  queries/           ✱ *.sql для sqlc
  sqlc.yaml, .air.toml, .golangci.yml
  go.mod, go.sum
  Dockerfile         multi-stage → distroless/static
.env.example
.dockerignore
.gitattributes       * text=auto eol=lf; *.sh eol=lf
.gitignore
.gitlab-ci.yml       ✱ в ТЗ опечатка: `.gilab-ci.yml`
biome.json, biome-plugins/
docker-compose.yml   ✱ dev: postgres + (опц.) redis; docker-compose.prod.yml
package.json         ✱ корневые скрипты (bun workspaces: client)
scripts/             ✱ slop-check.ts, new/ (генераторы), templates/ (шаблоны файлов)
CLAUDE.md, README.md
```

Почему `server/migrations` + `server/queries` вместо `server/drizzle/`: goose и sqlc
ожидают раздельные папки, а `drizzle/` вводил бы в заблуждение и людей, и модель.

**Лимит размера файлов — 1000 строк для любого файла, включая CSS.** В ЛифтДжобе
`main.css` дорос до **8 679 строк**, а потом его дважды перекраивали (12 файлов →
снова один): в обоих вариантах было неудобно. На хакатоне стили изначально
устроены так: токены — отдельный файл, компоненты — CSS Modules, раскладка —
утилиты Tailwind. Хук `file-size.ts` (п. 9) сам блокирует правку, которая выводит
файл за 1000 строк. Сгенерированные файлы и словари в исключениях.

---

## 4. Дизайн-система

### 4.1 Принципы (вынесены из ЛифтДжоба, проверены пользователями)

1. **Один шрифт, раздаётся с нашего сервера.** Пакет `@fontsource-variable/<шрифт>`
   (вариативный, любой вес 100–900). Google Fonts не подключаем: IP посетителя
   уходит в Google, это трансграничная передача ПДн по 152-ФЗ, которую пришлось бы
   описывать в политике. Шрифт задаёт **только** токен `--font`, новых `font-family`
   нет (в ЛифтДжобе стек был расписан в шести местах и разошёлся).
2. **Курсива нет** (`font-style: italic` ловит Grit-плагин). Второстепенный текст
   выделяем размером и `var(--text-muted)`.
3. **Ни одного браузерного контрола.** `<select>`, `alert/confirm/prompt`, нативный
   чекбокс и стандартные стили ссылок/кнопок не используются. Всё рисуют свои
   компоненты в `shared/ui`. Исключения — то, что браузер обязан показать сам:
   выбор файла (`<input type=file>` прячем за своей кнопкой), `<input type=date>`
   и системное меню копирования.
4. **Плоский дизайн, без теней.** `--shadow*: none`. Тень есть только у
   всплывающих поверхностей в тёмной теме (там у них нет другого способа
   отделиться от фона).
5. **Две темы с первого дня**, без мигания при загрузке (п. 4.6).
6. **Мобильная вёрстка — как приложение, а не сжатый сайт.**
7. **Никаких цветов в компонентах**, только токены. Цвет вне `tokens.css` ловит
   stylelint-правило / ревьюер.

### 4.2 Токены — `client/src/shared/styles/tokens.css`

Палитра ЛифтДжоба. Если у хакатона свой бренд, меняются только значения,
имена остаются.

```css
:root {
  color-scheme: light;
  --font: 'Roboto Variable', 'Roboto', 'Segoe UI', system-ui, -apple-system, sans-serif;

  /* палитра */
  --navy-900: #0a1f44;  --navy-800: #0f2d5e;  --navy-700: #153f7c;
  --blue-600: #2a78ff;  --blue-500: #2469d6;  --blue-100: #dce9fb;  --blue-50: #eef4fd;
  --teal-600: #0f9b8e;

  /* семантика — компоненты используют ТОЛЬКО эти */
  --bg: #ffffff;
  --surface: #ffffff;
  --surface-2: #f7f9fc;        /* hover, вторичный фон */
  --border: #e9e9e9;
  --divider: #cecece;
  --text: #111827;
  --text-muted: #979797;
  --primary: var(--blue-600);
  --primary-hover: #1c63e6;
  --on-primary: #ffffff;
  --danger: #ff3b30;   --danger-hover: #a93226;
  --success: #16a34a;
  --warning: #ff9300;  --warning-bg: #fff1da;
  --info-bg: #dae8ff;
  --overlay: rgba(0, 0, 0, .45);
  --page-mobile: #f2f4f8;      /* подложка под белыми карточками на телефоне */
  --shadow-sm: none; --shadow: none; --shadow-md: none;

  /* Радиусы и размеры текста от темы не зависят — они объявлены в app.css
     (@theme static) под именами Tailwind: --radius-xs 6 / sm 10 / md 15 / lg 20 /
     full 999px; --text-xs 12 / sm 13 / md 14 / base 15 / lg 16 / xl 20 / 2xl 24 / 3xl 32px.
     Объявить их здесь и сослаться из @theme inline нельзя: --radius-sm: var(--radius-sm)
     ссылается сам на себя, и браузер признаёт значение недействительным. */

  --space-1: 4px; --space-2: 8px; --space-3: 12px; --space-4: 16px;
  --space-5: 20px; --space-6: 24px; --space-8: 32px; --space-10: 40px;
  --content-max: 1200px;

  --dur-fast: .15s; --dur: .2s; --ease: cubic-bezier(.4, 0, .2, 1);
  --bp-mobile: 720px; --bp-header: 1000px; /* справочно: в @media переменные не работают */
}

[data-theme='dark'] {
  color-scheme: dark;
  /* нейтральный тёмно-серый: синий канал выше R максимум на 4–5 пунктов.
     Синяя «ночь» читалась как ночной режим карты, а не как тёмная тема */
  --bg: #0d0d0f;
  --surface: #17171a;
  --surface-2: #212124;
  --border: #2f2f34;
  --divider: #2f2f34;
  --text: #f0f0f2;
  --text-muted: #98979f;
  --primary: #4f8fee;
  --primary-hover: #6fa3f2;
  --success: #34c85f;          /* #16a34a на чёрном даёт контраст 3.5:1, мало */
  --warning: #ffb84d;
  --warning-bg: rgba(255, 147, 0, .16);
  --info-bg: rgba(79, 143, 238, .16);
  --danger-hover: #ff5a50;
  --blue-50: #1f2838;          /* «выделенное» состояние без светлых пятен */
  --blue-100: #24344d;
  --navy-800: #b4cdf5;         /* текст на --blue-100 */
  --overlay: rgba(0, 0, 0, .65);
  --page-mobile: var(--bg);
}

/* Всплывающие поверхности в тёмной теме — на ступень светлее карточки.
   Компонент ставит data-elevated, переопределение токенов сдвигает всё внутри. */
[data-theme='dark'] [data-elevated] {
  --surface: #1e1e22;
  --surface-2: #2a2a2f;
  --border: #36363c;
  box-shadow: 0 12px 32px rgba(0, 0, 0, .5);
}
```

Улучшение по сравнению с ЛифтДжобом: там у всплывающих поверхностей был длинный
`:is(.search-dropdown, .menu-dropdown, …)` из 20 классов, и про каждый новый поповер
нужно было помнить. Здесь хватает атрибута `data-elevated` на корне компонента.

### 4.3 Типографика

| Роль | Размер | Вес | Примечание |
|---|---|---|---|
| body | 15px / 1.5 | 400 | базовый |
| вторичный, подписи | 13px | 400 | `--text-muted` |
| мелкие метки | 12px | 500 | бейджи, счётчики |
| кнопка | 14px | 600 | `.btn-sm` — 13px |
| поле ввода | 15px desktop / **16px mobile** | 400 | iOS зумит страницу при фокусе на поле мельче 16px и не отдаляет |
| h3 / заголовок карточки | 16px | 600 | |
| h2 | 20px | 700 | `letter-spacing: -.3px` |
| h1 | 24px (лендинг — 32px) | 700 | `letter-spacing: -.3px` |
| деньги/числа | — | — | `font-variant-numeric: tabular-nums` (класс `.money`) |

Веса только 400/500/600/700. Различаем текст весом, размером и цветом, другим
начертанием — никогда.

### 4.4 Базовые стили (`base.css`)

С Tailwind v4 сброс браузерных стилей делает его preflight (margin, списки,
`img { display: block }`, `button` без фона, наследование шрифта у полей). В
`base.css` остаётся только то, чего в preflight нет: шрифт и фон body, фокус,
скрытые скроллбары, выделение, reduced-motion. Ниже полный вариант на случай
сборки без Tailwind; строки, которые дублируют preflight, при Tailwind удалить.

```css
*, *::before, *::after { box-sizing: border-box; }
html, body, * { scrollbar-width: none; }               /* ползунки скрыты, прокрутка работает */
*::-webkit-scrollbar { width: 0; height: 0; display: none; }
body {
  margin: 0; font-family: var(--font); font-size: var(--text-base); line-height: 1.5;
  background: var(--bg); color: var(--text);
  -webkit-font-smoothing: antialiased; text-rendering: optimizeLegibility;
}
button, input, textarea, select, optgroup { font: inherit; color: inherit; } /* иначе системный шрифт */
button { background: none; border: 0; padding: 0; cursor: pointer; }
a { color: var(--primary); text-decoration: none; }
a:hover { text-decoration: underline; }
h1, h2, h3, h4, p, ul, ol, figure { margin: 0; }
ul, ol { padding: 0; list-style: none; }
img, svg, video { display: block; max-width: 100%; }
:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
::selection { background: var(--blue-100); }
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: .01ms !important; transition-duration: .01ms !important; }
}
```

### 4.5 Компоненты: эталон

| Компонент | Спецификация |
|---|---|
| **Button** | `inline-flex`, gap 6, padding `8px 18px`, radius 6, 14px/600, `--primary` → hover `--primary-hover`, `transition: background .15s`, `white-space: nowrap`. Варианты: `secondary` (фон `--surface`, рамка `--border`, hover `--surface-2`), `danger`, `sm` (5px 12px, 13px). `disabled`: opacity .5 + `cursor: not-allowed` **+ причина в подсказке** |
| **Card** | `--surface`, `1px solid var(--border)`, radius 10, padding 20, без тени |
| **Input** | высота 40, radius 6, рамка `--border`, фокус: рамка `--primary`. Клик по подписи или паддингу фокусирует поле (`useFieldClickFocus` в ЛифтДжобе) |
| **Select** | свой: кнопка-триггер + панель `data-elevated`, клавиатура (↑↓ Enter Esc), `aria-expanded`, `role=listbox` |
| **Checkbox / Switch** | свои; выключенная настройка — **только с `disabledReason`** (текст под строкой + `title` + `aria-describedby`) |
| **Dialog / Confirm** | вместо `confirm()`; оверлей `--overlay`, radius 15, Esc закрывает, фокус заперт внутри, возврат фокуса |
| **Toast** | вместо `alert()`; снизу по центру, автоскрытие 4 с, `role=status` |
| **Empty** | по центру, `--text-muted`, padding 40px 0, 14px |
| **Error (поле)** | `--danger`, 13px, отступ 6px 0 |
| **Badge/Chip** | radius pill, 12px/500, фон `--blue-50`, текст `--primary` |
| **Avatar** | круг, без фото — инициалы на `--blue-100` цветом `--navy-800` |
| **Skeleton** | `--surface-2` с мерцанием (отключается при reduced-motion) |

**Страница `/ui` (витрина компонентов)** ✱. Все компоненты во всех состояниях и
в обеих темах на одной странице. Зачем: (1) жюри видит проработку, (2) ИИ-агент
и люди берут готовое, а не пишут своё, (3) `ui-auditor` сверяет с ней скриншоты.
В проде страницу закрываем (`noindex` и только в dev-сборке либо под флагом).

### 4.6 Тема без мигания

- Тема хранится в `localStorage['<app>_theme']` и в профиле пользователя. Инлайн-скрипт
  в `<head>` `index.html` ставит `data-theme` **до первой отрисовки** (иначе экран
  мигает белым). Хеш этого скрипта прописан в CSP (п. 8).
- Тема с сервера применяется, **только если в этом браузере выбора ещё не было**.
  Иначе ответ `/auth/me` затирал только что выбранную.
- Переключение темы идёт через View Transitions (`document.startViewTransition`,
  .45s), запасной вариант — временный класс `theme-fade` с transition цветов
  на `!important`.
- `color-scheme` на `:root` перекрашивает и редкие нативные элементы (скроллбары
  в полях, автозаполнение).

### 4.7 Мобильная вёрстка

- **Две точки перелома:** `720px` (всё) и `1000px` (шапка, ей нужно ~980px).
  С одной точкой планшет ломался так же, как телефон.
- **Проверка: `document.documentElement.scrollWidth === clientWidth`** на 375px на
  каждой странице. Одного широкого элемента хватает, чтобы «поехала» вся страница.
  Эту проверку автоматизирует Playwright-тест (п. 10).
- Вкладки шапки на телефоне уходят на свою строку (`order: 3; flex: 0 0 100%`) и
  прокручиваются вбок. Активная подтягивается `scrollIntoView({ block: 'nearest' })`.
- Высота полноэкранных экранов — `100dvh`, не `100vh`.
- Поля ввода на телефоне — 16px.
- **Inline-стиль медиазапросом не переопределить**, поэтому всё, что может понадобиться
  менять на мобильном, сразу оформляем классом.
- На телефоне: шапка экрана «‹ Заголовок» вместо хлебных крошек, подвал не под каждым
  экраном, а в настройках, основное действие — кнопка во всю ширину над нижней
  панелью, вход и регистрация без карточки-рамки, фон `--page-mobile` под белыми
  карточками.

---

## 5. Как писать стили (Tailwind v4 + CSS Modules)

Разделение обязанностей (запишите в `client/CLAUDE.md`):

| Что | Чем |
|---|---|
| раскладка, отступы, flex/grid, размеры, видимость по ширине | **утилиты Tailwind** в разметке |
| простой внешний вид (фон, текст, рамка, радиус из токенов) | **утилиты Tailwind** (`bg-surface text-muted rounded-sm`) |
| сложные состояния, анимации, псевдоэлементы, >~12 классов на элементе | **CSS Module** рядом с компонентом |
| повторяющийся набор классов | **компонент в `shared/ui`**, не `@apply` и не копипаста |
| значения | **только токены**: других цветов/шрифтов/радиусов в теме просто нет |

`client/src/shared/styles/app.css` — единственная точка входа стилей (Vite-плагин
`@tailwindcss/vite`, конфиг-файла `tailwind.config.js` в v4 нет). Три блока темы:

1. **`@theme` со сбросом** `--color-*`, `--font-*`, `--text-*`, `--radius-*`, `--shadow-*`,
   `--breakpoint-*`, `--container-*: initial` и точками перелома `md` = 721px, `lg` = 1001px.
   `bg-red-500`, `text-gray-700`, `font-serif`, `shadow-lg` после этого **не существуют**:
   Tailwind их не генерирует (проверено сборкой).
2. **`@theme static`** — радиусы и размеры текста (с `--text-*--line-height`). `static`
   оставляет переменные в `:root` всегда, поэтому CSS Modules берут `var(--radius-sm)`.
3. **`@theme inline`** — цвета и шрифт ссылками на токены: `--color-surface: var(--surface)`.
   `inline` вписывает `var(--surface)` прямо в утилиту, и смена `data-theme` перекрашивает
   всё без `dark:`. Вариант `dark:` всё же есть — `@custom-variant` по `data-theme`.

- Отступы — штатная шкала Tailwind (`--spacing: 0.25rem`): `p-2`=8px, `gap-2.5`=10px,
  `p-5`=20px — совпадает с ритмом ЛифтДжоба, трогать не нужно.
- **Классы проверяет Biome 2.6** (правила nursery, `tailwind` в `biome.json`):
  `noTailwindArbitraryValue` (`bg-[#f00]`, `w-[347px]`), `noTailwindRawColors`
  (`bg-red-500`), `useTailwindStaticClassStrings` (склейка `` `bg-${color}` `` — Tailwind
  таких классов не видит, нейросеть пишет их постоянно), `noTailwindLegacyUtilities`,
  `useTailwindSortedClasses` и `useTailwindShorthandClasses` (автопочинка `lint:fix`).
  Нужно произвольное значение — `{/* biome-ignore lint/nursery/noTailwindArbitraryValue: <причина> */}`
  строкой выше; slop-check требует, чтобы причина была.
- **Склейка классов — только `cx()`** из `@/shared/lib/cx` (`tailwind.mergeFunctions`):
  `class={cx(s.root, props.active && 'bg-accent-50', local.class)}`. Biome проверяет строки внутри.
- `@apply` не используем: это способ спрятать копипасту. Повторилось — компонент.
- В CSS Module токены — через `var(--…)`; `@reference '../styles/app.css'` в начале
  модуля, если внутри нужны `@variant md { … }` или `--spacing()`.

Компонент на Solid:

```tsx
// shared/ui/Button.tsx
import { splitProps, type JSX } from 'solid-js';
import s from './Button.module.css';

type Props = JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger';
  size?: 'md' | 'sm';
  disabledReason?: string;
};

export function Button(props: Props) {
  const [local, rest] = splitProps(props, ['variant', 'size', 'disabledReason', 'class']);
  return (
    <button
      type="button"
      {...rest}
      class={`${s.btn} ${s[local.variant ?? 'primary']} ${local.size === 'sm' ? s.sm : ''} ${local.class ?? ''}`}
      disabled={rest.disabled || !!local.disabledReason}
      title={local.disabledReason ?? rest.title}
    />
  );
}
```

Правила Solid, которые ИИ нарушает чаще всего (обязательно в `client/CLAUDE.md`):
- **Не деструктурировать props**, иначе теряется реактивность. Только `props.x`
  или `splitProps`/`mergeProps`.
- Списки — `<For>`/`<Index>`, условия — `<Show>`/`<Switch>`, не `.map()` и тернарники в JSX.
- Данные — `createResource` или `query`/`createAsync` из `@solidjs/router`;
  `createEffect` только для побочных эффектов, не для вычислений (`createMemo`).
- `class`, а не `className`; `onInput` для полей.
- Компонент выполняется **один раз**, ранний `return` при условии не работает,
  нужен `<Show>`.

---

## 6. Страница, подвал, куки, юр-документы

Базовая страница (задание из ТЗ) состоит из лендинга, подвала и юр-страниц.

**Подвал:** логотип, ссылки (О проекте, Пользовательское соглашение, Политика
конфиденциальности, Согласие на обработку ПДн, Политика cookie), контакты
`niktar411@yandex.ru` (ссылка `mailto:`), `+7 996 329-50-09` (`tel:+79963295009`),
© год. Ссылки на документы — функции сообщений (`label: () => m.x()`), а не
строки. Иначе подпись вычисляется один раз на языке первой отрисовки.

**Баннер cookie (по опыту ЛифтДжоба и требованиям 152-ФЗ):**
- Две **равнозначные** кнопки: «Только необходимые» и «Принять все»; ссылка
  «Подробнее» → `/cookies`. Кнопку отказа не прячем и не делаем серой: это тёмный
  паттерн.
- Обязательные куки (сессия) работают всегда, необязательные (аналитика, Sentry/
  GlitchTip, Метрика) **до согласия не грузятся вообще**. SDK подключаем через
  динамический `import()` только после `analyticsAllowed()`.
- Состояние согласия хранит модуль `shared/lib/consent.ts` (`readConsent/
  writeConsent/analyticsAllowed`), а не компонент баннера: его читает и код без UI.
- Баннер начинает в состоянии «неизвестно», чтобы не мигать у того, кто уже ответил.
- Выбор можно поменять в настройках или по ссылке «Настройки cookie» в подвале.

**Документы** (`/terms`, `/policy`, `/consent`, `/cookies`): оператор, состав
данных, цели, сроки, права субъекта, контакт для отзыва согласия (та же почта).
Отдельно перечислить, что шрифты и скрипты раздаются с собственного сервера, без
передачи третьим лицам. Все эти страницы **пререндерятся** (п. 7).

---

## 7. Производительность и SEO: пререндер, SSI, llms.txt

### 7.1 Пререндер публичных страниц

Работает как в ЛифтДжобе (`app/prerender.tsx` + `scripts/prerender.ts`), но на Solid:

1. Вторая сборка Vite в SSR-режиме: `vite-plugin-solid({ ssr: true })`,
   вход `src/app/prerender.tsx`. Он экспортирует список путей и
   `render(path) => renderToString(() => <App url={path} />)` из `solid-js/web`.
2. `scripts/prerender.ts` (Bun) для каждого пути кладёт
   `dist/prerender/<путь>/index.html`: это тот же `index.html` с разметкой страницы
   внутри `#root` и `generateHydrationScript()`.
3. Клиент: `hydrate()` для пререндеренных путей, `render()` для остальных.
4. nginx: `try_files /prerender$uri/index.html /index.html;`, прямой адрес
   `/prerender/…` отдаёт 404 (иначе дубль страницы).

Что пререндерим: лендинг, юр-страницы, «о проекте», `/ui` не пререндерим. Данные,
которые меняются (списки), отдаёт SSI.

### 7.2 SSI и `ssiAfterAssets`

- В `index.html` стоит `<!--# block name="seo_head" -->…<!--# endblock -->` со значениями
  по умолчанию и `<!--# include virtual="/api/seo/head" stub="seo_head" -->`.
- **`ssiAfterAssets`** — плагин Vite (только сборка, `transformIndexHtml` order `post`):
  переносит SSI-вставку **в конец `<head>`, после тегов CSS/JS/modulepreload**. nginx
  отдаёт SSI потоком: всё, что до вставки, уходит сразу, поэтому браузер начинает
  качать ассеты, не дожидаясь ответа API. Получается тот же выигрыш, что у 103 Early
  Hints, которые nginx сам не шлёт. Код плагина переносится из
  `liftjob/client/vite.config.ts` без изменений.
- **`inlineSmallCss`** встраивает CSS до 8 КБ (обычно это `@font-face`) прямо в HTML.
  Плюс `preload` файлов шрифта.
- nginx: для SSI-локаций `ssi on; brotli_static off; gzip_static off;` (SSI не
  разбирает сжатые файлы), `internal`-локация `/api/seo/head` с заголовком
  `X-Seo-Uri $request_uri` и таймаутом 3 с: если API лежит, остаётся блок по умолчанию.
- Go-сторона: `internal/seo` (разбор пути → title, description, canonical, JSON-LD),
  `/sitemap.xml`, `/robots.txt`. Кеш в памяти процесса на минуту допустим, потому что
  это копия данных, а не общее состояние.

### 7.3 `client/public/llms.txt`

Формат llmstxt.org: `# Название`, `> одна фраза о проекте`, абзац о том, что открыто
без входа, затем разделы `## …` со списками `- [Название](абсолютный URL): описание`,
в конце `## Optional` (юр-документы). Обновлять при каждой новой публичной странице,
вместе с regex в nginx, sitemap и robots.

### 7.4 Бюджет

- Главный JS < 60 КБ brotli (Solid это позволяет), CSS < 20 КБ.
- Словари i18n: по файлу на язык, грузятся `import()`. В ЛифтДжобе статический
  импорт всех языков занимал 355 из 685 КБ бандла.
- Картинки только в AVIF (WebP не создаём и не принимаем). В Go кодирование через
  `libvips` (`govips`), тяжёлое кодирование — через семафор (аналог `withEncodeSlot`).
- Lighthouse CI в пайплайне (п. 11) с порогами Performance ≥ 95, A11y ≥ 95.

---

## 8. nginx: HTTP/3, Brotli, защитные заголовки

Улучшение по сравнению с ЛифтДжобом: там заголовки повторялись в 5+ `location`, потому что
**`add_header` в `location` отменяет все `add_header` уровня `server`**. Здесь они
вынесены в сниппет, и каждая локация со своим `add_header` подключает его через `include`.

`client/snippets/security-headers.conf`:

```nginx
add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
add_header X-Content-Type-Options "nosniff" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header X-Frame-Options "DENY" always;
add_header Cross-Origin-Opener-Policy "same-origin" always;
add_header Cross-Origin-Resource-Policy "same-origin" always;
add_header Permissions-Policy "camera=(), microphone=(self), geolocation=(), payment=(), usb=(), interest-cohort=()" always;
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'sha256-<хеш инлайн-скрипта темы>'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; media-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests" always;
add_header Alt-Svc 'h3=":443"; ma=86400' always;
```

- Хеш инлайн-скрипта темы считает плагин Vite при сборке и подставляет в сниппет
  (или скрипт в Dockerfile). Руками не обновлять: поменяли скрипт — CSP молча его заблокирует.
- `style-src 'unsafe-inline'` нужен для `style=` в Solid; если инлайн-стилей нет —
  убрать.
- `server_tokens off;`, `listen 443 quic reuseport; listen 443 ssl;`, `http2 on;`,
  `ssl_protocols TLSv1.3 TLSv1.2;`, `ssl_early_data off` (защита от replay на POST).
- Brotli: `brotli_static on;` (сборка кладёт `.br` рядом), `brotli on; brotli_comp_level 4;`
  для JSON API. Модуль `ngx_brotli` собирается в `client/Dockerfile`.
- Кеш: `/assets/*` — `max-age=31536000, immutable`; `index.html` и пререндер — `no-cache`.
- Лимиты: `client_max_body_size`, `limit_req_zone` на `/api/auth/*`.
- Go дублирует заголовки безопасности для ответов API (middleware): если однажды
  API откроют мимо nginx, защита не пропадёт.
- Пара `nginx.conf` (dev) ↔ `nginx.prod.conf` меняется только вместе (правило-зеркало
  остаётся, хук напомнит).

---

## 9. Claude Code: CLAUDE.md, хуки, агенты, навыки

### 9.1 Принципы, которые сработали в ЛифтДжобе

- **Корневой `CLAUDE.md` короткий** (≤150 строк): команды, сквозные правила, запреты,
  таблица «где что описано». Подробности лежат в `docs/arch/*.md`, правило
  «прочитай нужный файл перед правкой подсистемы». Плюс `server/CLAUDE.md` и
  `client/CLAUDE.md`, которые подгружаются при работе в папке.
- Изменил поведение подсистемы — **обнови её `docs/arch/*.md`**, а не корневой файл.
- **Запреты ловят хуки и линтер**, а не только текст. Правило без автоматической
  проверки рано или поздно нарушают (`salaryText()` успел размножиться в три копии).
- Документация объясняет **«почему»** (история бага), тогда модель не откатывает решение.
- Подавление линтера — только построчно, с причиной (`// biome-ignore rule: why`,
  `//nolint:rule // why`).

### 9.2 Хуки (`.claude/hooks/*.ts`, Bun)

| Хук | Событие | Что делает |
|---|---|---|
| `session-start.ts` | SessionStart | сводка: ветка, незакоммиченное, поднят ли Postgres, последний пайплайн |
| `guard.ts` | PreToolUse (Edit/Write/Bash) | **deny**: правка миграции из текущего HEAD `server/migrations/*.sql` (не проверка применения в БД); `npm/pnpm/yarn/npx`; правка сгенерированных файлов (`schema.d.ts`, `internal/db/*.go`, `api/openapi.json`) с подсказкой «запусти `bun run gen:api` / `sqlc generate`»; `git push --force` |
| `file-size.ts` ✱ | PreToolUse (Edit/Write) | считает строки будущего файла; > 1000 → deny с советом разбить (gen и словари в исключениях) |
| `lint.ts` | PostToolUse (Edit/Write) | `.ts/.tsx/.css` → `biome lint <файл>`; `.go` → `gofumpt -l` + `golangci-lint run --fast-only <пакет>`; ошибки уходят модели |
| `gen-reminder.ts` ✱ | PostToolUse | правка `internal/**/types.go` → «перегенерируй API»; правка `queries/*.sql` → «`sqlc generate`»; правка `migrations/` → «примени и `sqlc generate`» |
| `mirrors.ts` | PostToolUse | только для оставшихся пар (nginx dev↔prod); остальное закрывает OpenAPI |
| `prompt-router.ts` ✱ | UserPromptSubmit | подмешивает к промту нужный навык и Definition of Done (п. 9.6) |
| `slop-guard.ts` ✱ | PreToolUse (Edit/Write) | deny правки с заглушками, `any`, сырыми цветами, произвольными классами Tailwind и т. п. (п. 9.6) |
| `done-gate.ts` ✱ | Stop | не даёт закончить ход, пока в diff есть мусор или ошибки линта/типов (п. 9.6) |
| `typecheck.ts` | Stop | по изменённым воркспейсам: `tsc --noEmit` (client), `go build ./... && go vet ./...` (server) |
| `after-push.ts` | PostToolUse (Bash) | после `git push` в main предлагает `/watch-deploy` |

Ответ PreToolUse-хука на запрет:
`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"…"}}`.
Команды запрещаются только в позиции команды (regex `(^|[;&|(\n])\s*npm\b`), иначе
хук срабатывает на слово «npm» в тексте коммита.

`settings.json`: allow для безопасных команд (`bun run lint/test/build`, `go test`,
`go vet`, `git status/diff/log`, `docker ps/logs`), deny для `git push --force`, `Read(~/.ssh/**)`, `.env`.

### 9.3 Агенты (`.claude/agents/*.md`)

Субагенты нужны для длинного вывода, не для мелочей.

| Агент | Модель | Задача |
|---|---|---|
| **test-runner** | haiku | lint → typecheck/vet → build → тесты; возвращает только сводку и первые ошибки |
| **reviewer** | sonnet | инварианты проекта по `git diff` (не общие баги): права на ресурс, цена не из тела запроса, `ctx` в каждом I/O, нет утечки горутин, миграция есть при смене схемы, `gen:api` выполнен, i18n без литералов, токены вместо цветов, мобильные правила. Код не правит, ≤30 строк находок |
| **translator** | sonnet | заменяет заглушки в словарях после `i18n:sync` |
| **ui-auditor** ✱ | sonnet | открывает изменённые страницы в браузере на 375/768/1280 в обеих темах, проверяет `scrollWidth`, контраст, системные контролы, сверяет с `/ui`; возвращает список с адресами |

### 9.4 Навыки (`.claude/skills/<name>/SKILL.md`)

- `/check`: то же, что CI.
- `/new-route`: huma-операция (`huma.Register` с `OperationID`, тегами, `Security`),
  типы в `types.go`, сервис, тест через `humatest`, `bun run gen:api`.
- `/new-page`: папка `features/<домен>`, маршрут, i18n, мобильная проверка,
  пререндер/SSI, если страница публичная (+ llms.txt, sitemap, nginx regex).
- `/migration`: `goose create x sql` → правка → `goose up` на dev → `sqlc generate`.
- `/i18n`: ключ → `i18n:sync` → перевод.
- `/seed` ✱: наполнить базу демо-данными и демо-аккаунтами (п. 12).
- `/prod-status`, `/watch-deploy`: пайплайн и `/api/health`.

### 9.5 Каркас корневого `CLAUDE.md`

```md
# CLAUDE.md
<Проект> — <одна фраза>. SolidJS + TS (Vite, Tailwind v4, CSS Modules) / Go (chi + huma, sqlc + pgx, goose, slog) / PostgreSQL / nginx (HTTP/3, Brotli).

## Команды
bun install · bun run dev · bun run lint · bun run gen:api · bun run test · bun run migrate · bun run seed

## Сквозные правила
- Контракт API — только Go-типы → `bun run gen:api`. Сгенерированное руками не правим.
- Цвета, радиусы, шрифт — только токены (`shared/styles/tokens.css`). Шрифт один, курсива нет.
- Никаких браузерных контролов: Select/Checkbox/Dialog/Toast из `shared/ui`.
- Строки UI — `m.key()`, без литералов. Файлы ≤ 1000 строк.
- Go: `ctx` первым аргументом в каждом I/O; ошибки оборачиваем `fmt.Errorf("…: %w", err)`;
  `slog.InfoContext(ctx, "msg", "key", v)`; никаких `fmt.Println`/`log.Print`.
- Цена/права/роль — только с сервера, никогда из тела запроса.
- Новая публичная страница → пререндер или SSI + sitemap + robots + llms.txt + regex nginx (оба конфига).

## Где что описано
| docs/arch/design-system.md | токены, компоненты, мобильная вёрстка |
| docs/arch/backend.md | слои, ошибки, транзакции, новый маршрут |
| … |
```

### 9.6 Защита от мусора: чтобы и «тупой» промт давал нормальный код

Цель: тиммейт пишет «сделай страницу профиля» — и получает код по шаблону, без
заглушек, выдуманных цветов и «// остальная логика тут». Текст в CLAUDE.md
модель иногда игнорирует, **хуки — никогда**. Поэтому защита в шесть слоёв, от
мягкого к жёсткому:

```
промт ─▶ ① prompt-router (подмешивает навык и правила)
       ─▶ ② .claude/rules/* (правила по типу файла)
       ─▶ ③ генераторы new:* (скелет по шаблону, не «с нуля»)
       ─▶ ④ slop-guard (deny правки с мусором — до записи на диск)
       ─▶ ⑤ done-gate (не даёт закончить ход, пока есть заглушки/ошибки)
       ─▶ ⑥ тот же slop-check в pre-push и CI (для тех, кто пишет без Claude)
```

**① `prompt-router.ts` (UserPromptSubmit).** Смотрит на промт по ключевым словам и
добавляет в контекст (`hookSpecificOutput.additionalContext`), какой навык
использовать и что считается готовым:

| Слова в промте | Подмешивается |
|---|---|
| страниц, экран, раздел, форм | «Используй навык /new-page; компоненты только из shared/ui; проверка на 375px» |
| эндпоинт, ручк, api, маршрут | «Навык /new-route; после — `bun run gen:api`» |
| таблиц, колонк, поле в базе, миграц | «Навык /migration; старые миграции не трогать» |
| текст, перевод, надпись | «Навык /i18n» |
| стил, цвет, красив, дизайн | «Только токены и классы темы; смотри /ui и docs/arch/design-system.md» |
| всегда | Definition of Done (ниже), коротко |

Промт короче ~8 слов без явной цели — добавляет: «Задача расплывчатая: выбери
ближайший шаблон, в конце перечисли сделанные допущения».

**② `.claude/rules/*.md` — правила по путям.** Файл с frontmatter `paths:`
подгружается, только когда модель трогает подходящие файлы, — правила всегда
под рукой и не раздувают корневой CLAUDE.md:

```md
---
paths: ["client/**/*.tsx"]
---
- props не деструктурировать; <For>/<Show>; class, не className
- компонент есть в shared/ui → использовать его, свой не писать
- классы только из темы (bg-surface, text-muted, rounded-sm); произвольных [..] нет
- строки только m.key(); данные только через api-клиент
```

Аналогично `go.md` (`server/**/*.go`: ctx, обёртка ошибок, slog, слои),
`sql.md` (`server/queries/**`, `server/migrations/**`), `styles.md` (`**/*.css`).

**③ Генераторы вместо «напиши с нуля».** `bun run new:page profile`,
`new:component Avatar`, `new:route jobs`, `new:migration add_jobs` — Bun-скрипты в
`scripts/new/` кладут файлы из шаблонов `scripts/templates/` (правильные импорты,
i18n, CSS Module, тест, регистрация маршрута). Навыки начинаются с вызова
генератора, модель только **дописывает логику** в готовый скелет. Чем меньше
модель пишет с чистого листа, тем меньше она «изобретает».

**④ `slop-guard.ts` (PreToolUse на Edit/Write) — главный фильтр.** Проверяет
**только добавляемый текст** (`new_string` / `content`), поэтому старый код
не блокирует. Нашёл — `deny` с причиной и правильной заменой; модель сама
переделывает.

| Что ловим | Где | Чем заменить (текст в причине отказа) |
|---|---|---|
| `TODO`, `FIXME`, `XXX`, `HACK` | везде | доделай сейчас или скажи пользователю, что не сделано |
| `// ...`, `/* ... */`, `// остальной код`, `// rest of`, `// implement` | везде | напиши код полностью |
| `not implemented`, `throw new Error('TODO`, `panic("TODO` / `panic("not implemented` | TS, Go | реализуй или убери вызов |
| `lorem ipsum`, `example.com`, `John Doe`, `test@test`, `mockData`, `fakeData`, `dummy` | вне `*_test`, `*.test.*`, `seed/` | реальные данные — из API; демо — в сиды |
| `setTimeout(` + `resolve` (фейковая задержка), захардкоженный массив вместо запроса | `client/src/features` | `createResource`/`query` к API |
| `as any`, `: any`, `<any>`, `@ts-ignore`, `@ts-nocheck` | TS | правильный тип из `schema.d.ts` |
| `biome-ignore`/`//nolint`/`@ts-expect-error` **без причины** после `:` | везде | указать правило и причину |
| пустой `catch {}`, `catch (e) { console…}` | TS | `toast.error(…)` / проброс |
| `_ = err`, `if err != nil { return nil }` (ошибка потеряна) | Go | `return fmt.Errorf("…: %w", err)` |
| `fmt.Print*`, `log.Print*`, `console.*` | код | `slog` / убрать |
| `context.Background()`/`TODO()` вне `cmd/` и тестов | Go | `ctx` из аргумента |
| `#[0-9a-f]{3,8}`, `rgb(`, `hsl(` | `.tsx`, `*.module.css` (кроме `tokens.css`) | токен `var(--…)` / класс темы |
| произвольное значение / палитра / склеенный класс Tailwind | `.tsx` | ловит Biome (`noTailwind*`, `useTailwindStaticClassStrings`) — класс из темы, склейка через `cx()` |
| `style={{` с цветом/размером шрифта | `.tsx` | класс |
| `font-family`, `italic`, `<select`, `alert(`/`confirm(`, голый `fetch(` | клиент | см. п. 4.1 |
| кириллица в JSX-тексте/атрибутах `placeholder`/`title`/`aria-label` | `.tsx` | `m.key()` + `bun run i18n:sync` |
| новый файл в `shared/ui/` с именем, похожим на существующий (`Button2`, `MyButton`, `CustomSelect`) | клиент | расширь существующий компонент |

Вся логика — в **одном** скрипте `scripts/slop-check.ts` (правила — массив
`{ re, files, except, fix }`), хук его только вызывает:

Реализация — `scripts/slop-check.ts` (массив `RULES`, функции `checkContent`,
`checkNewPath`, `scanWorkingTree`, `scanRange`), тесты на каждое правило и на
ложные срабатывания — `scripts/slop-check.test.ts`. Новое правило добавляется туда
вместе с двумя тестами: «ловит» и «не трогает нормальный код».

**⑤ `done-gate.ts` (Stop).** Перед концом хода прогоняет `slop-check` по всему
`git diff` + быстрый линт/типы изменённых воркспейсов. Есть проблемы → отвечает
`{"decision":"block","reason":"<список>"}` — модель **не может закончить**, пока
не исправит. Защита от зацикливания: если во входе `stop_hook_active: true`
(хук уже блокировал этот ход) — пропускает и просит в причине честно перечислить
нерешённое пользователю.

**⑥ Те же проверки без Claude.** `.githooks/pre-push` и CI-задание `slop` гоняют
`bun scripts/slop-check.ts --diff origin/main...HEAD`. Тиммейт с Cursor, Copilot или
вручную получает тот же отказ — правила одни для всех.

**Definition of Done** (в корневой CLAUDE.md, его же подмешивает prompt-router):
1. Функция работает полностью: ни заглушек, ни моков, ни «потом доделаем».
2. Все состояния: загрузка (скелетон), пусто, ошибка (toast), успех.
3. Тексты через i18n, вид через токены и `shared/ui`, вёрстка проверена на 375px.
4. Сервер: права проверены, ошибки обёрнуты, `gen:api` выполнен, есть тест.
5. Не уверен в требовании — выбери вариант по шаблону и **в конце ответа перечисли
   допущения**. Не сделал часть — **скажи прямо**, не маскируй заглушкой.

**Права (`settings.json`):** `ask` для `bun add*`, `go get*`, `rm -rf*`,
`docker compose down*` — новая зависимость и удаление только с подтверждения
человека; `deny` для `git push --force*`, `Read(./.env*)`, `Read(~/.ssh/**)`.
`deny` из общего `settings.json` не перебивается личным `settings.local.json`,
поэтому защита работает у всей команды, если файл закоммичен.

---

## 10. Бэкенд на Go: соглашения

- **Слои:** `handler` (huma: вход/выход, коды) → `service` (логика, права) →
  `db` (sqlc). Handler не ходит в базу напрямую.
- **Ошибки:** `huma.Error404NotFound("…")` и т. п. в handler; доменные ошибки —
  `errors.Is/As` с переводом в HTTP в одном месте. Текст ошибки для пользователя
  передаётся кодом i18n (`"code": "job.not_found"`), клиент переводит его сам.
- **Конфиг:** `internal/config` (env → struct, `caarlos0/env`), падает при старте без
  обязательных переменных (`JWT_SECRET`, `DATABASE_URL`). Порт — `API_PORT`.
- **Сервер:** `http.Server` с `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`,
  `IdleTimeout`; graceful shutdown по SIGTERM (`signal.NotifyContext` →
  `srv.Shutdown(ctx)`), чтобы деплой не рвал запросы.
- **Middleware:** request-id → recover → slog-логирование запроса (метод, путь,
  статус, длительность, request_id) → security headers → CORS (только dev) →
  rate-limit (`httprate`, ключ — userID или IP) → auth.
- **Аутентификация:** JWT в httpOnly `Secure` `SameSite=Lax` куке, отдельный секрет
  для админки. Пароли через `argon2id`.
- **Транзакции:** хелпер `db.Tx(ctx, pool, func(q *db.Queries) error)`.
- **Фоновые задачи:** одна горутина-планировщик с `ctx`, при нескольких репликах
  `pg_try_advisory_lock` выбирает лидера (аналог `isLeader`).
- **Логи:** `slog` JSON в проде, `tint` в dev; уровень из `LOG_LEVEL`. Секреты и ПДн
  не логируем.
- **`/api/health`:** пинг пула с таймаутом 1 с; CI проверяет его после деплоя.
- **Тесты:** `humatest` для handler'ов; интеграционные — testcontainers-go с
  Postgres 18 (без стенда тест **падает**, а не пропускается: тихий пропуск хуже
  падения). Имя тестовой базы обязано содержать `test` (предохранитель от TRUNCATE
  по dev-базе).
- **Линт:** `golangci-lint` v2 с `errcheck, govet, staticcheck, gosec, revive,
  contextcheck, noctx, bodyclose, sqlclosecheck, forbidigo` (запрет `fmt.Print*`,
  `log.*`).

---

## 11. CI/CD (GitLab)

```yaml
stages: [check, build, deploy, verify]

variables:
  GIT_DEPTH: "20"

.bun:   { image: oven/bun:1, cache: { key: { files: [bun.lock] }, paths: [node_modules/, client/node_modules/] }, before_script: [bun install --frozen-lockfile] }
.go:    { image: golang:1-alpine, cache: { key: { files: [server/go.sum] }, paths: [.go/pkg/mod] }, variables: { GOPATH: "$CI_PROJECT_DIR/.go" } }

lint:client:   { extends: .bun, stage: check, script: [bun run lint] }
lint:go:       { image: golangci/golangci-lint:latest, stage: check, script: [cd server && golangci-lint run] }
contract:      { extends: [.bun], stage: check, script: [bun run gen:api, git diff --exit-code api/ client/src/shared/api/] }
test:go:
  extends: .go
  stage: check
  services: [{ name: postgres:18-alpine, alias: postgres }]
  variables: { POSTGRES_DB: app_test, POSTGRES_PASSWORD: postgres, TEST_DATABASE_URL: "postgres://postgres:postgres@postgres:5432/app_test" }
  script: [cd server && go test -race ./...]
build:client:  { extends: .bun, stage: check, script: [bun run --filter client build, bun run --filter client test], artifacts: { paths: [client/dist/] } }

build:images:  # docker buildx → GitLab Container Registry, тег = $CI_COMMIT_SHORT_SHA
  stage: build
  rules: [{ if: $CI_COMMIT_BRANCH == "main" }]

review:        # ✱ review app на каждый MR: https://mr-$CI_MERGE_REQUEST_IID.<домен>
  stage: deploy
  environment: { name: review/$CI_MERGE_REQUEST_IID, url: "https://mr-$CI_MERGE_REQUEST_IID.<домен>", on_stop: stop_review }
  rules: [{ if: $CI_PIPELINE_SOURCE == "merge_request_event" }]

deploy:prod:   # ssh → docker compose pull && up -d → goose up → проверка /api/health, иначе откат на прошлый тег
  stage: deploy
  environment: { name: production }
  rules: [{ if: $CI_COMMIT_BRANCH == "main" }]

slop:          { extends: .bun, stage: check, script: [git fetch origin main --depth=50, bun scripts/slop-check.ts --diff origin/main...HEAD] }  # ✱ п. 9.6
lighthouse:    { stage: verify, script: [bunx @lhci/cli autorun], rules: [{ if: $CI_COMMIT_BRANCH == "main" }] }
smoke:e2e:     { stage: verify, script: [bunx playwright test --project=smoke] }  # ✱ + проверка scrollWidth на 375px
```

- **Review apps на каждый MR** ✱: на хакатоне это главный ускоритель. Каждый в команде
  смотрит и показывает свою ветку по ссылке, не мешая `main`.
- **Автооткат:** если `/api/health` не ответил за 60 с, деплой поднимает прошлый тег.
- Секреты — переменные GitLab (protected, masked, `File` для ключей).
- `.githooks/pre-push` повторяет `check` и **громко** предупреждает, если
  интеграционные тесты не прогонялись (не молчит).
- GitLab на сервере доступен через AmneziaWG: раннер на той же машине, реестр по
  HTTP внутри VPN помечен `insecure-registries` у демона сборки.

---

## 12. Что ещё сделает проект сильнее на хакатоне

1. **Сиды и демо-аккаунты** (`bun run seed`, навык `/seed`): реалистичные данные
   с первого часа. Пустой экран на защите — минус баллы. Логины демо-аккаунтов
   лежат в `server/seed/README.md`, не в чате.
2. **Демо-режим** (`DEMO_MODE=true`): кнопка «Войти как …» на странице входа, сброс
   данных по ночам. Жюри не регистрируется.
3. **Страница `/ui`** (п. 4.5): витрина проработки и защита от «самодельных» компонентов.
4. **Одна команда для старта:** `bun run dev` поднимает `docker compose up -d postgres`,
   миграции, сиды, air и Vite. Новый участник работает через 5 минут.
5. **Скелетоны и пустые состояния** у каждого списка, оптимистичные обновления
   (Solid `action` + `useSubmission`). Интерфейс воспринимается быстрее, чем есть.
6. **Состояния ошибок:** ErrorBoundary снаружи всех провайдеров, свои 404/500,
   офлайн-баннер. Белого экрана быть не должно.
7. **Сбор ошибок** (GlitchTip, самохостинг, совместим с Sentry SDK) только после
   согласия на cookie; `tracesSampleRate: 0`, без PII.
8. **Доступность:** клавиатура везде, `:focus-visible`, `aria-*` на своих контролах,
   контраст ≥ 4.5:1 (проверено в обеих темах), `prefers-reduced-motion`.
9. **PWA-минимум:** манифест, иконки, `offline.html`. Установка на телефон хорошо
   смотрится на демо.
10. **README для жюри:** скриншоты обеих тем и мобильной версии, ссылка на прод и
    review app, схема архитектуры, демо-логины, «что сделано за N часов».
11. **Feature flags** через env (`FEATURE_X=1`): недоделанное прячется перед защитой
    без отката кода.
12. **Changelog из коммитов:** коммиты по-русски в повелительном виде, по одному
    смыслу; `git log --oneline` за хакатон читается как история проекта.

---

## 13. Чек-лист первого дня

- [ ] Сервер: пользователь `deploy`, SSH только по ключу, `ufw` (22, 80, 443/tcp+udp, порт AWG), fail2ban, автообновления безопасности
- [ ] AmneziaWG: GitLab и реестр слушают только внутри VPN
- [ ] GitLab CE в Docker + runner (docker executor), аккаунты команды (без общего логина), 2FA, защищённая `main` (только MR)
- [ ] Репозиторий со структурой из п. 3, `.gitattributes` (`eol=lf`), `.env.example`
- [ ] `tokens.css`, `app.css` (Tailwind + `@theme`), шрифт self-hosted, инлайн-скрипт темы
- [ ] `scripts/slop-check.ts` + хуки `slop-guard`, `done-gate`, `prompt-router`, генераторы `new:*`, `.claude/rules/` (п. 9.6)
- [ ] `shared/ui`: Button, Input, Select, Checkbox, Switch, Dialog, Toast, Card + страница `/ui`
- [ ] Лендинг + подвал (контакты, документы) + баннер cookie + 4 юр-страницы, пререндер
- [ ] Go: `cmd/api`, config, slog, health, graceful shutdown, huma + OpenAPI, первая миграция
- [ ] `bun run gen:api` и проверка контракта в CI
- [ ] nginx: HTTP/3, Brotli, сниппет заголовков, CSP с хешем, SSI-блок, `llms.txt`, `robots.txt`, `sitemap.xml`
- [ ] `.gitlab-ci.yml`: check → build → deploy → verify, review apps
- [ ] `.claude/`: CLAUDE.md ×3, хуки (guard, file-size, lint, gen-reminder, typecheck), агенты, навыки
- [ ] `.githooks/pre-push`, `git config core.hooksPath .githooks` у каждого
- [ ] Сиды и демо-аккаунты
- [ ] Проверить: securityheaders.com — A+, Lighthouse ≥ 95, `scrollWidth` на 375px, обе темы
