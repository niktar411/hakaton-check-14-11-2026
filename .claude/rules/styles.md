---
paths:
  - "client/**/*.css"
  - "client/**/*.tsx"
---

# Стили: Tailwind v4 + CSS Modules + токены

Вход стилей — `client/src/shared/styles/app.css`. Токены — `tokens.css`. Дизайн-система — `docs/playbook.md` п. 4–5.

**Чем что делать:**
- раскладка, отступы, flex/grid, размеры, адаптив → классы Tailwind в разметке;
- простой вид → классы темы: `bg-surface`, `bg-surface-2`, `text-text`, `text-muted`, `text-primary`,
  `border-border`, `rounded-xs|sm|md|lg|full`, `text-xs|sm|md|base|lg|xl|2xl|3xl`;
- сложные состояния, анимации, псевдоэлементы, больше ~12 классов → `Компонент.module.css` рядом;
- повторяющийся набор классов → компонент в `shared/ui` (не `@apply`, не копипаста).

**Нельзя (ловят Biome и хуки):**
- стандартная палитра (`bg-red-500`, `text-gray-700`) — её нет в теме, класс не сработает;
- произвольные значения `bg-[#f00]`, `w-[347px]` — если правда нужно, строкой выше
  `{/* biome-ignore lint/nursery/noTailwindArbitraryValue: <причина> */}`;
- склеенные классы `` `bg-${color}` `` — Tailwind их не видит. Полные строки по условию:
  `classList={{ 'bg-danger': error() }}` или выбор из объекта целых классов;
- сырые цвета `#fff`, `rgb()`, `hsl()` вне `tokens.css`;
- `font-family` (кроме `var(--font)`), курсив, `@apply`, захардкоженные значения в `style={{…}}`.

**Адаптив:** mobile-first. Без префикса — телефон, `md:` — от 721px, `lg:` — от 1001px (шапка).
Проверка: `document.documentElement.scrollWidth === clientWidth` на 375px. Поля ввода на телефоне —
`text-lg` (16px), иначе iOS зумит. Полноэкранное — `h-dvh`, не `h-screen`.

**Тёмная тема** переключается токенами сама. `dark:` нужен редко. Всплывающая поверхность
(меню, поповер, модалка, тост) — атрибут `data-elevated` на корне.

**Фокус** — общий `:focus-visible` из base.css; свой outline не убирать.
