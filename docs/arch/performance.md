# Производительность фронтенда и инфраструктуры

Что делает сайт быстрым, где это реализовано и как проверять. Правила не
дублируют CLAUDE.md, а описывают механику конкретных подсистем.

## Сборка клиента (Vite)

- `client/vite.config.ts`: `build.target = "baseline-widely-available"` —
  компактный вывод без лишних полифиллов.
- `manualChunks` делит `node_modules` на долгоживущие чанки: `solid`,
  `paraglide`, `vendor`. Код приложения меняется, чанк-хэши зависимостей
  стабильны — повторные визиты берут их из кэша.
- Все маршруты, кроме лендинга, подключены через `lazy()` из
  `@solidjs/router` (`client/src/app/main.tsx`): экраны SAM, planner, chat,
  admin и legal страницы грузятся по требованию.
- `client/vite-plugins/compress.ts` — плагин без зависимостей: после сборки
  пишет `.gz`-сиблинги для каждого сжимаемого файла в `dist/` (zlib, level 9)
  и отчёт `dist/bundle-report.json` с размерами raw/gz. Крупнейшие чанки
  печатаются в лог сборки. Роль отчёта — вместо rollup-plugin-visualizer.
- **Brotli не собирается**: в образе `nginx:1.29.3-alpine` нет ngx_brotli,
  файлы `.br` были бы балластом. Если появится brotli-образ — выдавать `.br`
  в плагине рядом с `.gz` и включить `brotli_static on` в
  `client/nginx.conf`.

## nginx

- `gzip_static on` (`client/nginx.conf`) — отдаёт готовые `.gz` вместо
  компрессии на лету; `gzip on` остаётся фоллбэком для динамики.
- `open_file_cache` кэширует метаданные статических файлов.
- `access_log ... buffer=32k flush=5s` — запись журнала блоками.
- HTTP/3: `listen 443 quic` во всех TLS-серверах
  (`client/nginx.vhosts.conf.template`), `reuseport` — только один, на
  первом листенере порта (правило nginx). `http2 on` сохранён на TCP.
  `Alt-Svc` рекламирует h3 браузерам.
- UDP 443 проброшен в контейнер (`docker-compose.yml`, `443/udp`) — без
  этого QUIC не работает, проверьте firewall хоста.
- Иконки `dragon*/logo*` — `expires 30d`; `/assets/` — 1 год + immutable;
  `index.html` — `no-store`, чтобы хэши чанков всегда подхватывались.

## Критический путь первой загрузки

- Тема применяется до первой отрисовки: `client/index.html` содержит
  инлайн-скрипт чтения `portal-theme` (раньше — блокирующий запрос
  `/theme-boot.js`; файл оставлен для совместимости).
- В `#root` лежит статический сплэш «Загрузка…» — первая отрисовка до
  доставки JS; Solid заменяет содержимое при гидрации.
- CSP в двух местах: `client/snippets/security-headers.conf` (nginx) и
  `server/internal/operations/http.go` (API отдаёт `/admin/login`).
  Инлайн-скрипт темы разрешён **по sha256-хэшу** — при его изменении
  пересчитайте оба CSP, иначе страница не загрузится (CSP блок). Хэш
  считается от точного текста элемента:
  `openssl dgst -binary -sha256 | openssl base64`.
  Инлайн-`<style>` разрешён через `style-src 'unsafe-inline'` (стандарт
  playbook): Tailwind v4 перезаписывает инлайн-стили при сборке, хэш бы
  расходился. Также сниппет несёт `Alt-Svc`: `location = /index.html`
  определяет собственные `add_header` и не наследует серверные.

## Кэширование публичного API

- `server/internal/operations/cache.go` — middleware `cachePublic`:
  успешные GET публичной группы и `sitemap.xml` получают
  `Cache-Control: public, max-age=60, stale-while-revalidate=300` и
  сильный ETag (sha256 тела, буфер до 1 MiB); `If-None-Match` даёт 304 без
  тела. Ошибки и стримы (логин, чат, вебхуки) не кэшируются.
- Через nginx ответы сжаты (gzip), поэтому ETag в браузере может
  отсутствовать — nginx снимает ETag при компрессии; расчёт на
  `Cache-Control` + 304 для клиентов без сжатия и для прокси-уровня.
- Тесты: `server/internal/operations/cache_test.go`.

## Индексы БД

- Миграция `00012_add_sam_hotpath_indexes.sql`: композитные индексы под
  комплаенс-запросы — `sam_installations(product_id, device_name)`,
  `sam_licenses(product_id, status, expires_at)`,
  `sam_products(section_code, class_code)`.
- Проверка планов: `EXPLAIN (ANALYZE, BUFFERS)` на
  `SamComplianceOverview` и на публичный каталог.

## Измерения

- Размер бандла: `bun run build:client` печатает топ-10 чанков по gz и
  пишет `dist/bundle-report.json` — смотреть в PR перед merge.
- Lighthouse: `npx lighthouse https://<host>/ --preset=desktop --view`
  (и профиль мобильного throttling по умолчанию). Гонять по горячей
  инстанции после деплоя, не на dev-сервере с холодным кэшем.
- Полевые Core Web Vitals: Search Console → Core Web Vitals report
  (данные CrUX, нужен трафик).
- Проверка HTTP/3: `curl -I --http3 https://<host>/` или вкладка Protocol
  в DevTools Network (`h3`).

## Что осознанно не сделано

- Пререндер публичных страниц не внедрён: для честного HTML-вывода нужен
  headless-рендер или SSI-шаблоны; сейчас компенсируется сплэшем в `#root`.
- keyset-пагинация вместо `LIMIT/OFFSET` не переписана: объёмы демо-данных
  не показывают деградации, внедрять при первом реальном лаге списков.
- Keyset и EULA-анализатор — вне зоны этой ветки.
