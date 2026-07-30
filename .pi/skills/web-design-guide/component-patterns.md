# Component Patterns

Disclosed reference for the web-design-guide skill. Each pattern is self-contained — copy, adapt, compose.

## Base Layout

The root template that wraps every page. Loads CDN dependencies, sets up dark mode, and provides the shell.

```templ
package layouts

import "github.com/agentic-demo/platform/web/templates/components"

templ Base(title string, content templ.Component) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>{ title }</title>
			<link rel="preconnect" href="https://fonts.googleapis.com"/>
			<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin=""/>
			<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet"/>
			<script src="https://cdn.tailwindcss.com"></script>
			<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
			<script src="https://unpkg.com/htmx.org@2.0.4"></script>
			<!-- tailwind.config and :root / .dark CSS from SKILL.md go here -->
		</head>
		<body
			class="bg-surface-page text-content font-sans antialiased min-h-screen"
			x-data="{ dark: localStorage.getItem('theme') === 'dark' }"
			x-init="$watch('dark', v => {
				document.documentElement.classList.toggle('dark', v)
				localStorage.setItem('theme', v ? 'dark' : 'light')
			})"
			x-bind:class="{ dark: dark }"
		>
			@components.Navbar()
			<main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				@content
			</main>
		</body>
	</html>
}
```

Initialize dark mode from `localStorage` on page load to prevent flash of wrong theme:
```html
<script>
  if (localStorage.getItem('theme') === 'dark' ||
      (!localStorage.getItem('theme') && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    document.documentElement.classList.add('dark')
  }
</script>
```
Place this inline `<script>` immediately after `<html>`, before `<head>`, to avoid FOUC.

## Buttons

### Primary Button
```html
<button class="inline-flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-on shadow-sm hover:bg-primary-hover focus:ring-2 focus:ring-primary focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
  Save Changes
</button>
```

### Secondary Button
```html
<button class="inline-flex items-center justify-center rounded-lg border border-border bg-surface-card px-4 py-2 text-sm font-medium text-content shadow-sm hover:bg-surface-page focus:ring-2 focus:ring-primary focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
  Cancel
</button>
```

### Danger Button
```html
<button class="inline-flex items-center justify-center rounded-lg bg-intent-error px-4 py-2 text-sm font-medium text-white shadow-sm hover:opacity-90 focus:ring-2 focus:ring-intent-error focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
  Delete
</button>
```

### Ghost Button (icon-only)
```html
<button aria-label="Close" class="inline-flex items-center justify-center rounded-lg p-2 text-content-secondary hover:bg-surface-page hover:text-content focus:ring-2 focus:ring-primary focus:ring-offset-2 transition-colors">
  <svg class="h-5 w-5" ...></svg>
</button>
```

### HTMX Button with Loading
```html
<button hx-delete="/sources/123"
        hx-target="closest tr"
        hx-swap="outerHTML"
        hx-confirm="Delete this source?"
        class="inline-flex items-center gap-2 rounded-lg bg-intent-error px-3 py-1.5 text-sm font-medium text-white hover:opacity-90 focus:ring-2 focus:ring-intent-error focus:ring-offset-2 disabled:opacity-50">
  <span class="htmx-indicator:hidden">Delete</span>
  <span class="htmx-indicator:flex hidden items-center gap-1">
    <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
    </svg>
    Deleting...
  </span>
</button>
```

## Forms

### Input Field
```templ
templ InputField(id string, label string, inputType string, value string, required bool, placeholder string) {
	<div class="space-y-1">
		<label for={ id } class="block text-sm font-medium text-content">
			{ label }
			if required {
				<span class="text-intent-error" aria-hidden="true">*</span>
			}
		</label>
		<input
			id={ id }
			name={ id }
			type={ inputType }
			value={ value }
			placeholder={ placeholder }
			required={ required }
			class="block w-full rounded-lg border border-border bg-surface-card px-3 py-2 text-sm text-content placeholder:text-content-muted focus:border-primary focus:ring-2 focus:ring-primary/20 focus:outline-none transition-colors"
		/>
	</div>
}
```

### Form with HTMX Submission
```html
<form hx-post="/sources"
      hx-target="#result"
      hx-indicator="#form-spinner"
      class="space-y-4 max-w-lg">
  @InputField("name", "Source Name", "text", "", true, "My database")
  @InputField("url", "Connection URL", "url", "", true, "postgres://...")

  <div class="flex items-center gap-3 pt-2">
    <button type="submit" class="... primary button classes ...">
      Create Source
    </button>
    <div id="form-spinner" class="htmx-indicator">
      <svg class="h-5 w-5 animate-spin text-primary" viewBox="0 0 24 24" fill="none">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
    </div>
  </div>
</form>
<div id="result"></div>
```

### Client-Side Validation with Alpine
```html
<div x-data="{ email: '', submitted: false }">
  <form @submit="submitted = true" hx-post="/register" class="space-y-4">
    <div class="space-y-1">
      <label for="email" class="block text-sm font-medium text-content">Email</label>
      <input
        id="email"
        name="email"
        type="email"
        x-model="email"
        required
        class="block w-full rounded-lg border px-3 py-2 text-sm ..."
        x-bind:class="{ 'border-intent-error': submitted && !email }"
      />
      <p x-show="submitted && !email" class="text-sm text-intent-error" x-cloak>
        Email is required
      </p>
    </div>
    <button type="submit" class="... primary button ...">Register</button>
  </form>
</div>
```

Add `[x-cloak] { display: none !important }` to the base CSS to prevent flash of unstyled Alpine content.

## Cards

### Standard Card
```templ
templ Card(title string, body templ.Component) {
	<div class="bg-surface-card border border-border rounded-lg shadow-sm p-6">
		<h3 class="text-lg font-medium text-content mb-4">{ title }</h3>
		@body
	</div>
}
```

### Interactive Card (HTMX)
```html
<a hx-get="/sources/123"
   hx-target="#detail-panel"
   hx-swap="innerHTML"
   class="block bg-surface-card border border-border rounded-lg shadow-sm p-6 hover:border-primary hover:shadow-md cursor-pointer transition-all">
  <h3 class="text-lg font-medium text-content">PostgreSQL Production</h3>
  <p class="text-sm text-content-secondary mt-1">Last synced 2 hours ago</p>
</a>
```

### Stat Card
```templ
templ StatCard(label string, value string, change string, positive bool) {
	<div class="bg-surface-card border border-border rounded-lg shadow-sm p-6">
		<p class="text-sm font-medium text-content-secondary">{ label }</p>
		<p class="text-2xl font-bold text-content mt-1">{ value }</p>
		if positive {
			<p class="text-sm text-intent-success mt-2">↑ { change }</p>
		} else {
			<p class="text-sm text-intent-error mt-2">↓ { change }</p>
		}
	</div>
}
```

## Alerts

### Inline Alert
```templ
templ Alert(intent string, message string) {
	<div
		class="rounded-lg border p-4 text-sm"
		x-data="{ visible: true }"
		x-show="visible"
		x-transition
	>
		<div class="flex items-start justify-between gap-3">
			<div class="flex items-start gap-3">
				if intent == "success" {
					<div class="bg-surface-card border-intent-success">
						<svg class="h-5 w-5 text-intent-success mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
						</svg>
					</div>
				}
				if intent == "error" {
					<div class="bg-surface-card border-intent-error">
						<svg class="h-5 w-5 text-intent-error mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/>
						</svg>
					</div>
				}
				if intent == "warning" {
					<div class="bg-surface-card border-intent-warning">
						<svg class="h-5 w-5 text-intent-warning mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
						</svg>
					</div>
				}
				if intent == "info" {
					<div class="bg-surface-card border-intent-info">
						<svg class="h-5 w-5 text-intent-info mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
						</svg>
					</div>
				}
				<p class="text-content">{ message }</p>
			</div>
			<button @click="visible = false" aria-label="Dismiss" class="text-content-muted hover:text-content transition-colors">
				<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/>
				</svg>
			</button>
		</div>
	</div>
}
```

Apply intent-specific border and background:
- Success: `border-intent-success/20 bg-intent-success/5`
- Error: `border-intent-error/20 bg-intent-error/5`
- Warning: `border-intent-warning/20 bg-intent-warning/5`
- Info: `border-intent-info/20 bg-intent-info/5`

### Toast (HTMX Out-of-Band)

Return a toast fragment from the handler after an action:

```go
if isHTMX(c) {
    c.Response().Header().Set("HX-Trigger", "show-toast")
    return c.NoContent(http.StatusOK)
}
```

Listen with Alpine on the layout:

```html
<div x-data="{ show: false, message: '', intent: 'success' }"
     x-on:show-toast.window="show = true; message = $event.detail.message; intent = $event.detail.intent; setTimeout(() => show = false, 4000)"
     x-show="show"
     x-transition
     x-cloak
     class="fixed bottom-4 right-4 z-50 max-w-sm">
  <div class="bg-surface-raised border border-border rounded-lg shadow-lg p-4 text-sm text-content"
       x-bind:class="{
         'border-l-4 border-l-intent-success': intent === 'success',
         'border-l-4 border-l-intent-error': intent === 'error'
       }">
    <p x-text="message"></p>
  </div>
</div>
```

## Navigation

### Top Navbar
```templ
package components

templ Navbar() {
	<nav class="border-b border-border bg-surface-card" x-data="{ mobileOpen: false }">
		<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
			<div class="flex h-16 items-center justify-between">
				<div class="flex items-center gap-8">
					<a href="/" class="text-xl font-bold text-primary">Platform</a>
					<div class="hidden sm:flex items-center gap-1">
						<a href="/sources" class="px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page transition-colors">Sources</a>
						<a href="/reports" class="px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page transition-colors">Reports</a>
						<a href="/settings" class="px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page transition-colors">Settings</a>
					</div>
				</div>
				<div class="flex items-center gap-2">
					@ThemeToggle()
					<button @click="mobileOpen = !mobileOpen" class="sm:hidden p-2 rounded-lg text-content-secondary hover:bg-surface-page" aria-label="Toggle menu">
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/>
						</svg>
					</button>
				</div>
			</div>
		</div>
		<div x-show="mobileOpen" x-transition class="sm:hidden border-t border-border">
			<div class="px-4 py-3 space-y-1">
				<a href="/sources" class="block px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page">Sources</a>
				<a href="/reports" class="block px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page">Reports</a>
				<a href="/settings" class="block px-3 py-2 rounded-lg text-sm font-medium text-content-secondary hover:text-content hover:bg-surface-page">Settings</a>
			</div>
		</div>
	</nav>
}
```

### Theme Toggle
```templ
templ ThemeToggle() {
	<button
		@click="dark = !dark"
		aria-label="Toggle dark mode"
		class="p-2 rounded-lg text-content-secondary hover:bg-surface-page hover:text-content transition-colors"
	>
		<svg x-show="!dark" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/>
		</svg>
		<svg x-show="dark" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" x-cloak>
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/>
		</svg>
	</button>
}
```

## Tables

### Data Table with HTMX Pagination
```templ
templ DataTable(headers []string, rows []templ.Component) {
	<div class="overflow-x-auto">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border">
					for _, h := range headers {
						<th class="text-left py-3 px-4 font-medium text-content-secondary">{ h }</th>
					}
				</tr>
			</thead>
			<tbody id="table-body">
				for _, row := range rows {
					@row
				}
			</tbody>
		</table>
	</div>
	<div
		id="table-pagination"
		hx-get="/sources?page=2"
		hx-trigger="revealed"
		hx-swap="afterend"
		hx-target="#table-body"
	>
		<div class="flex justify-center py-4">
			<div class="htmx-indicator">
				<svg class="h-5 w-5 animate-spin text-primary" viewBox="0 0 24 24" fill="none">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
			</div>
		</div>
	</div>
}
```

### Table Row
```templ
templ TableRow(name string, status string, updatedAt string, id string) {
	<tr class="border-b border-border hover:bg-surface-page transition-colors">
		<td class="py-3 px-4 text-content font-medium">{ name }</td>
		<td class="py-3 px-4">
			<span
				class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
				if status == "active" {
					class="bg-intent-success/10 text-intent-success"
				} else {
					class="bg-content-muted/10 text-content-muted"
				}
			>
				{ status }
			</span>
		</td>
		<td class="py-3 px-4 text-content-secondary text-xs">{ updatedAt }</td>
		<td class="py-3 px-4 text-right">
			<button hx-delete={ "/sources/" + id }
			        hx-target="closest tr"
			        hx-swap="outerHTML"
			        hx-confirm="Delete this source?"
			        class="text-content-muted hover:text-intent-error transition-colors"
			        aria-label={ "Delete " + name }>
				<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
				</svg>
			</button>
		</td>
	</tr>
}
```

## Modal

### Alpine.js Modal
```html
<div x-data="{ open: false }">
  <button @click="open = true" class="... primary button ...">Open Modal</button>

  <div x-show="open"
       x-transition:enter="transition ease-out duration-200"
       x-transition:enter-start="opacity-0"
       x-transition:enter-end="opacity-100"
       x-transition:leave="transition ease-in duration-150"
       x-transition:leave-start="opacity-100"
       x-transition:leave-end="opacity-0"
       class="fixed inset-0 z-50 flex items-center justify-center p-4"
       x-cloak>
    <!-- Backdrop -->
    <div class="absolute inset-0 bg-black/50" @click="open = false"></div>
    <!-- Dialog -->
    <div
      x-show="open"
      x-transition:enter="transition ease-out duration-200"
      x-transition:enter-start="opacity-0 scale-95"
      x-transition:enter-end="opacity-100 scale-100"
      x-transition:leave="transition ease-in duration-150"
      x-transition:leave-start="opacity-100 scale-100"
      x-transition:leave-end="opacity-0 scale-95"
      @keydown.escape.window="open = false"
      role="dialog"
      aria-modal="true"
      class="relative bg-surface-raised border border-border rounded-xl shadow-lg p-6 w-full max-w-md"
    >
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold text-content">Confirm Action</h2>
        <button @click="open = false" aria-label="Close" class="p-1 rounded-lg text-content-muted hover:text-content transition-colors">
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/>
          </svg>
        </button>
      </div>
      <p class="text-sm text-content-secondary mb-6">Are you sure you want to proceed?</p>
      <div class="flex justify-end gap-3">
        <button @click="open = false" class="... secondary button ...">Cancel</button>
        <button @click="open = false" class="... danger button ...">Confirm</button>
      </div>
    </div>
  </div>
</div>
```

## Tabs

```html
<div x-data="{ active: 'overview' }">
  <div class="border-b border-border">
    <nav class="flex gap-1 -mb-px" aria-label="Tabs">
      <template x-for="tab in ['overview', 'settings', 'logs']" :key="tab">
        <button
          @click="active = tab"
          x-text="tab.charAt(0).toUpperCase() + tab.slice(1)"
          class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors"
          x-bind:class="active === tab
            ? 'border-primary text-primary'
            : 'border-transparent text-content-secondary hover:text-content hover:border-border-strong'"
        ></button>
      </template>
    </nav>
  </div>
  <div class="py-6">
    <div x-show="active === 'overview'">
      <!-- overview content -->
    </div>
    <div x-show="active === 'settings'">
      <!-- settings content -->
    </div>
    <div x-show="active === 'logs'">
      <!-- logs content -->
    </div>
  </div>
</div>
```

For tabs that load content from the server, combine HTMX with Alpine:
```html
<button @click="active = 'logs'"
        hx-get="/sources/123/logs"
        hx-target="#tab-logs"
        hx-trigger="click once"
        hx-swap="innerHTML">
  Logs
</button>
<div id="tab-logs" x-show="active === 'logs'">
  <div class="flex justify-center py-8 text-content-muted">Loading...</div>
</div>
```

## Empty States

```templ
templ EmptyState(icon templ.Component, title string, description string, action templ.Component) {
	<div class="flex flex-col items-center justify-center py-16 px-4 text-center">
		<div class="text-content-muted mb-4">
			@icon
		</div>
		<h3 class="text-lg font-medium text-content mb-1">{ title }</h3>
		<p class="text-sm text-content-secondary mb-6 max-w-sm">{ description }</p>
		@action
	</div>
}
```

## Loading & Transition Patterns

### HTMX Loading Indicator
```html
<!-- Spinner shown during any HTMX request within the parent -->
<div class="htmx-indicator flex items-center gap-2 text-sm text-content-muted">
  <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
  </svg>
  Loading...
</div>
```

Style the indicator in base CSS:
```css
.htmx-indicator { display: none; }
.htmx-request .htmx-indicator { display: inline-flex; }
.htmx-request.htmx-indicator { display: inline-flex; }
```

### Skeleton Loading
```templ
templ Skeleton() {
	<div class="animate-pulse space-y-3">
		<div class="h-4 bg-border rounded w-3/4"></div>
		<div class="h-4 bg-border rounded w-1/2"></div>
		<div class="h-4 bg-border rounded w-5/6"></div>
	</div>
}
```

### HTMX Swap Transition
```css
/* Fade-in on HTMX content swap */
.htmx-settling { opacity: 0; }
.htmx-settling { transition: opacity 150ms ease-in; }
```
