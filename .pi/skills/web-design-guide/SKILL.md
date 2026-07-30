---
name: web-design-guide
description: Project design system — palette, typography, layout, and component conventions. Use when creating or modifying templates, components, pages, styling, or any UI work with templ, HTMX, Alpine.js, or Tailwind.
---

# Web Design Guide

Project-specific design system for Go + a-h/templ + HTMX + Alpine.js + Tailwind CSS (CDN).

For accessibility and UX linting after building UI, invoke the `web-design-guidelines` skill.

## Tailwind Setup

Use the Play CDN. Place in the base layout `<head>`:

```html
<script src="https://cdn.tailwindcss.com"></script>
<script>
  tailwind.config = {
    darkMode: 'class',
    theme: {
      extend: {
        colors: {
          surface: {
            page: 'var(--color-surface-page)',
            card: 'var(--color-surface-card)',
            raised: 'var(--color-surface-raised)',
          },
          content: {
            DEFAULT: 'var(--color-content)',
            secondary: 'var(--color-content-secondary)',
            muted: 'var(--color-content-muted)',
            inverse: 'var(--color-content-inverse)',
          },
          primary: {
            DEFAULT: 'var(--color-primary)',
            hover: 'var(--color-primary-hover)',
            subtle: 'var(--color-primary-subtle)',
            on: 'var(--color-primary-on)',
          },
          border: {
            DEFAULT: 'var(--color-border)',
            strong: 'var(--color-border-strong)',
          },
          intent: {
            success: 'var(--color-intent-success)',
            warning: 'var(--color-intent-warning)',
            error: 'var(--color-intent-error)',
            info: 'var(--color-intent-info)',
          },
        },
        fontFamily: {
          sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
          mono: ['JetBrains Mono', 'ui-monospace', 'monospace'],
        },
      },
    },
  }
</script>
```

## Palette

Two themes driven by CSS custom properties on `:root` (light) and `.dark` (dark). Toggle by adding/removing the `dark` class on `<html>`.

### Light Theme (`:root`)

```css
:root {
  --color-surface-page: #f8fafc;
  --color-surface-card: #ffffff;
  --color-surface-raised: #ffffff;

  --color-content: #0f172a;
  --color-content-secondary: #475569;
  --color-content-muted: #94a3b8;
  --color-content-inverse: #ffffff;

  --color-primary: #4f46e5;
  --color-primary-hover: #4338ca;
  --color-primary-subtle: #eef2ff;
  --color-primary-on: #ffffff;

  --color-border: #e2e8f0;
  --color-border-strong: #cbd5e1;

  --color-intent-success: #059669;
  --color-intent-warning: #d97706;
  --color-intent-error: #e11d48;
  --color-intent-info: #0284c7;
}
```

### Dark Theme (`.dark`)

```css
.dark {
  --color-surface-page: #020617;
  --color-surface-card: #0f172a;
  --color-surface-raised: #1e293b;

  --color-content: #f1f5f9;
  --color-content-secondary: #94a3b8;
  --color-content-muted: #64748b;
  --color-content-inverse: #020617;

  --color-primary: #818cf8;
  --color-primary-hover: #a5b4fc;
  --color-primary-subtle: #1e1b4b;
  --color-primary-on: #020617;

  --color-border: #1e293b;
  --color-border-strong: #334155;

  --color-intent-success: #34d399;
  --color-intent-warning: #fbbf24;
  --color-intent-error: #fb7185;
  --color-intent-info: #38bdf8;
}
```

### Token Usage

| Token | Tailwind class | Purpose |
|-------|---------------|---------|
| `surface-page` | `bg-surface-page` | Page background |
| `surface-card` | `bg-surface-card` | Cards, panels, containers |
| `surface-raised` | `bg-surface-raised` | Modals, dropdowns, popovers |
| `content` | `text-content` | Body text, headings |
| `content-secondary` | `text-content-secondary` | Captions, labels, helper text |
| `content-muted` | `text-content-muted` | Placeholders, timestamps, disabled |
| `primary` | `bg-primary` / `text-primary` | CTAs, links, active states |
| `primary-subtle` | `bg-primary-subtle` | Selected rows, badges, highlights |
| `border` | `border-border` | Dividers, card borders |
| `intent-*` | `text-intent-success` etc. | Semantic feedback colors |

Shadows use `shadow-sm` (cards) and `shadow-lg` (modals/dropdowns) — both are dark-mode-aware via Tailwind defaults.

## Typography

Font: **Inter** (sans) for UI, **JetBrains Mono** (mono) for code. Load Inter from Google Fonts in the base layout.

| Role | Class | Usage |
|------|-------|-------|
| Page title | `text-3xl font-bold tracking-tight` | One per page |
| Section heading | `text-xl font-semibold` | Section separators |
| Card title | `text-lg font-medium` | Card headers |
| Body | `text-base` | Default body text |
| Small | `text-sm` | Captions, table cells, form labels |
| Tiny | `text-xs` | Badges, timestamps, meta |

Line heights: `leading-relaxed` for body paragraphs, `leading-tight` for headings and dense UI.

## Layout

- **Page container**: `max-w-7xl mx-auto px-4 sm:px-6 lg:px-8`
- **Content width**: `max-w-3xl` for reading content, `max-w-5xl` for data-dense views
- **Vertical rhythm**: `py-8` page padding, `space-y-6` between sections, `gap-4` or `gap-6` in grids
- **Cards**: `bg-surface-card border border-border rounded-lg shadow-sm p-6`
- **Responsive grid**: `grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6`

### Breakpoint Strategy

| Breakpoint | Width | Purpose |
|-----------|-------|---------|
| Default | < 640px | Mobile — single column, stacked |
| `sm` | 640px+ | Tablet — two columns, sidebar appears |
| `lg` | 1024px+ | Desktop — full layout, three columns |
| `xl` | 1280px+ | Wide — max-width cap, extra whitespace |

## Interaction Stack

Three tools with distinct responsibilities. Each handles what it does best.

### HTMX — Server Interactions

HTMX drives all server communication. Use it for anything that touches the backend.

- Data fetching, form submission, CRUD operations
- Pagination, filtering, search
- Server-rendered partial updates (`hx-swap`, `hx-target`)
- Optimistic UI via `hx-swap-oob` for out-of-band updates

```html
<button hx-post="/sources"
        hx-target="#source-list"
        hx-swap="beforeend"
        hx-indicator="#spinner">
  Create Source
</button>
```

Detect HTMX requests in handlers:
```go
func isHTMX(c echo.Context) bool {
    return c.Request().Header.Get("HX-Request") == "true"
}
```

Return HTML fragments for HTMX requests, full pages for regular navigation.

### Alpine.js — Client Interactions

Alpine handles state that never needs a server round-trip.

- Dropdowns, modals, tabs, accordions
- Form validation feedback (client-side)
- Theme toggling (dark/light mode)
- Local UI state (`x-data`, `x-show`, `x-transition`)

```html
<div x-data="{ open: false }" class="relative">
  <button @click="open = !open">Options</button>
  <div x-show="open"
       x-transition
       @click.outside="open = false"
       class="absolute right-0 mt-2 w-48 bg-surface-raised border border-border rounded-lg shadow-lg">
    <a href="/settings" class="block px-4 py-2 text-sm text-content hover:bg-primary-subtle">Settings</a>
  </div>
</div>
```

Load Alpine.js from CDN in the base layout:
```html
<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
```

### templ — Structure

templ templates provide type-safe HTML structure. Rules:

- Components receive typed Go values — no `any`, no `map[string]any`
- Keep logic in templates to conditionals and loops only
- Extract repeated patterns into templ components, not CSS `@apply`
- Compose: `Layout` wraps `Page`, `Page` composes `Card`, `Card` contains `Button`
- Organize under `web/templates/` as `layouts/`, `components/`, `pages/`

## Guardrails

- Every interactive element has a visible focus ring: `focus:ring-2 focus:ring-primary focus:ring-offset-2`
- Every icon-only button carries `aria-label`
- Every form input has an associated `<label>` with matching `for`/`id`
- Color is never the sole indicator — pair intent colors with icons or text
- Loading states use `hx-indicator` (HTMX) or `x-cloak` (Alpine) — never leave the user staring at a frozen UI
- Disabled states use `opacity-50 cursor-not-allowed` plus `disabled` attribute
- Prefer semantic HTML elements: `<nav>`, `<main>`, `<section>`, `<article>`, `<aside>`, `<header>`, `<footer>`

## Component Patterns

For detailed component examples — base layout, buttons, forms, cards, alerts, navigation, modals, loading states, and common HTMX/Alpine patterns — read [component-patterns.md](component-patterns.md).
