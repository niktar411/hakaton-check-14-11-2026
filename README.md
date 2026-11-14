# Проект хакатона

SolidJS + TypeScript (Vite, Tailwind v4, CSS Modules) · Go (chi + huma, sqlc + pgx, goose, slog) ·
PostgreSQL 18 · nginx (HTTP/3, Brotli, SSI) · Gitea + Gitea Actions.

## Быстрый старт

```bash
bun install
bun run hooks:install   # pre-push: lint, slop-check, типы, тесты
bun run lint && bun run slop && bun run typecheck && bun run test
```

## Как здесь работать

- Правила для людей и нейросети — `CLAUDE.md`; полный свод (стек, дизайн-система, nginx, CI) — `docs/playbook.md`.
- Новое — генераторами: `bun run new:page | new:component | new:route | new:migration`.
- Заглушки, моки, `any`, сырые цвета, произвольные классы Tailwind, русский текст в JSX
  не проходят: их ловят хуки Claude Code (`.claude/hooks/`), Biome, pre-push и CI —
  правила одни для всех (`scripts/slop-check.ts`).

## Состояние каркаса

Готово: правила и защита (`.claude/`, `scripts/`, `biome.json`, `.githooks/`), токены и тема
Tailwind (`client/src/shared/styles/`). Дальше по чек-листу `docs/playbook.md` п. 13:
клиент на Vite + Solid, сервер на Go, nginx, `.gitea/workflows/ci.yml`.

## Командная работа

Общие инструкции для разных ИИ-клиентов: AGENTS.md.
Доска, статусы и ревью: docs/team-workflow.md. Шаблон: docs/task-template.md.
Проверка каркаса одной командой: bun run check.
