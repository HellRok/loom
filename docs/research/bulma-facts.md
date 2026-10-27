# Bulma facts and class-collision audit

Research for Loom #40, part of the map #39, "Migrate the web UI to Bulma". Researched 2026-10-27.

Sources:
- **npm**: `npm view bulma` and the `bulma-1.0.4.tgz` tarball from registry.npmjs.org.
- **dist**: the CSS files inside that tarball (`package/css/…`). "dist line N" means a line of the unminified `css/bulma.css`.
- **docs**: bulma.io/documentation pages, linked inline.
- **GitHub**: github.com/jgthms/bulma releases and the LICENSE file.

## Version, file, licence

- **Latest is 1.0.4**, released 2025-04-19. It is npm `latest` and GitHub's "Latest" release (`gh release list -R jgthms/bulma`). Nothing has shipped since then.
- **File to vendor: `css/bulma.min.css`** from the npm package. The jsDelivr URL is `https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css` ([installation](https://bulma.io/documentation/start/installation/)).
  - It is 677,931 bytes raw (about 662 KiB) and 65,998 bytes gzipped.
  - Its sha256 is `67fa26df1ca9e95d8f2adc7c04fa1b15fa3d24257470ebc10cc68b9aab914bee`.
  - The file header reads `/*! bulma.io v1.0.4 | MIT License | github.com/jgthms/bulma */`.
  - The versioned name `bulma-1.0.4.min.css` would match how `static/` names htmx.
- **Alternative builds** ship in `css/versions/` ([alternative versions](https://bulma.io/documentation/start/alternative-versions/)):

  | File (min) | What changes | Raw bytes | Gzip bytes |
  |---|---|---|---|
  | `bulma.min.css` | complete (library + helpers) | 677,931 | 65,998 |
  | `versions/bulma-no-dark-mode.min.css` | light theme only | 556,058 | 53,924 |
  | `versions/bulma-no-helpers.min.css` | no helper classes | 473,100 | n/a |
  | `versions/bulma-prefixed.min.css` | every class gets a `bulma-` prefix | 726,625 | n/a |
  | `versions/bulma-no-helpers-prefixed.min.css` | no helpers, with the prefix | 510,058 | n/a |

- **Licence: MIT**, "Copyright (c) 2023 Jeremy Thomas" ([LICENSE](https://github.com/jgthms/bulma/blob/main/LICENSE)). npm also lists `"license": "MIT"`.
- **No JavaScript**: "The Bulma package does not come with any JavaScript" ([navbar docs](https://bulma.io/documentation/components/navbar/)).

## Dark mode

- **It is automatic.** `bulma.min.css` has one `@media (prefers-color-scheme: dark)` block that redefines the `--bulma-*` variables on `:root` (dist line 1785). It also has a `prefers-color-scheme: light` block (dist line 917). The [dark mode docs](https://bulma.io/documentation/features/dark-mode/) say the same.
- **To force dark**, put `data-theme="dark"` or `class="theme-dark"` on `<html>`, or on any element to scope it (docs, and dist line 2736).
- **To force light**, put `data-theme="light"` or `class="theme-light"` on the element. The dist defines `[data-theme=light], .theme-light` (dist line 1835), but the docs page doesn't mention it. This works because an attribute or class selector on `<html>` beats a `:root` media-query rule: same specificity, later in the file.
- **To disable dark mode entirely**, vendor `versions/bulma-no-dark-mode.min.css`. A grep of that file finds no `prefers-color-scheme` and no `data-theme` rules.
- **What this means for Loom**:
  - Our `loom.css` already has its own `prefers-color-scheme: dark` block, so Bulma's automatic dark mode lines up with it.
  - Any of our own colours that should follow the theme need dark values too. Alternatively, define them in terms of `--bulma-*` variables.

## Theming without Sass

- **Every variable starts with `--bulma-`.** Global variables live on `:root`. Components set their own variables on the component selector, and a more specific scope wins ([CSS variables](https://bulma.io/documentation/features/css-variables/)). A theme is just a set of variables on `:root`, `[data-theme=x]` or `.theme-x` ([themes](https://bulma.io/documentation/features/themes/)).
- **Colours are stored as separate H, S and L parts**, then combined. From the dist `:root`:
  - **Primary/accent**:
    - `--bulma-primary-h` (default `171deg`), `--bulma-primary-s` (`100%`), `--bulma-primary-l` (`41%`) make up `--bulma-primary`.
    - Links use `--bulma-link-h/-s/-l` (`233deg 100% 63%`). Plain `a` gets `color: var(--bulma-link-text)`, which is `hsl(link-h, link-s, link-on-scheme-l)`.
    - The other named colours follow the same pattern: `info`, `success`, `warning`, `danger`.
  - **Background**: the whole palette is tinted by `--bulma-scheme-h` and `--bulma-scheme-s` (`221`, `14%`).
    - `--bulma-scheme-main-l: 100%` gives the page and card background (`--bulma-scheme-main`).
    - `--bulma-background-l: 96%` gives `--bulma-background`, used for `code`, `pre` and the like.
    - `html` takes `background-color: var(--bulma-body-background-color)`.
  - **Text**:
    - `--bulma-text-h/-s/-l` (`221deg 14% 29%`) make up `--bulma-text`.
    - `--bulma-text-strong-l: 21%` is for headings and `strong`.
    - `--bulma-text-weak-l: 48%` is the natural match for our `--muted`.
    - `body` uses `--bulma-body-color`.
  - **Borders**: `--bulma-border-l: 86%` gives `--bulma-border`, and `--bulma-border-weak-l: 93%` gives `--bulma-border-weak`. Both use the scheme hue and saturation.
- **Caveat: changing L doesn't recompute the shades.** The shade ladder is a list of precomputed literals, such as `--bulma-primary-00-l: 1%` … `--bulma-primary-90-l: 91%`. The `-light`, `-dark` and `-invert` variants point at those fixed steps. Changing `-h` or `-s` moves every shade. Changing `-l` moves only the base colour. The dark theme also sets its own `*-on-scheme-l` values.
- **Extra colours without Sass**: Sass generates the `is-<colour>` modifiers, so CSS alone can't add a new colour name to every component. The modifiers only set component variables, though, so we can copy that pattern for the components we use. The dist rule for a tag is:

  ```css
  .tag.is-primary { --bulma-tag-h: var(--bulma-primary-h); --bulma-tag-s: var(--bulma-primary-s);
                    --bulma-tag-background-l: var(--bulma-primary-l); --bulma-tag-color-l: var(--bulma-primary-invert-l); }
  ```

  So a hand-written `.tag.is-teal { --bulma-tag-h: 172deg; --bulma-tag-s: 83%; --bulma-tag-background-l: 34%; --bulma-tag-color-l: 100%; }` would give us a teal tag. Our 8-colour label pool (`internal/store/labels.go`) needs this, because Bulma has no orange, teal, purple or pink.

## `.content` (rendered markdown)

The [content docs](https://bulma.io/documentation/elements/content/) list `p`, `ul`, `ol`, `dl`, `h1`–`h6`, `blockquote`, `em`, `strong` and `table`, with example output for `pre`, `figure`, `img`, `sup` and `sub`. The dist rules under `.content` cover:

- **Headings**:
  - `h1`–`h6` are `--bulma-text-strong` and extrabold, at sizes 2em, 1.75em, 1.5em, 1.25em, 1.125em and 1em.
  - Each has a bottom margin, plus a top margin when it isn't the first child.
- **Paragraphs and blocks**: `p`, `dl`, `ol`, `ul`, `blockquote`, `pre` and `table` get a bottom margin when they aren't the last child.
- **Lists**:
  - `ul` gets `disc outside` with a 2em indent. Nested lists switch to `circle`, then `square`.
  - `ol` is `decimal`. It can be changed with `is-lower-alpha`, `is-lower-roman`, `is-upper-alpha` and `is-upper-roman`.
  - Consecutive `li` items are 0.25em apart, and `dd` is indented 2em.
- **Tables**: `td` and `th` get a cell border and padding. `th` gets the heading colour and `text-align: inherit`. `thead` and `tfoot` cells get heavier borders, and the last row of `tbody` has no bottom border.
- **Code**:
  - `.content pre` scrolls horizontally with `white-space: pre` and its own padding.
  - Bulma styles `code` and `pre` globally, outside `.content` too.
    - `code` gets `--bulma-code` colour (the danger hue, so red), `--bulma-code-background`, a 0.5em radius and padding.
    - `pre` gets `--bulma-pre-background`, `--bulma-pre`, scrolling and `white-space: pre`.
    - `pre code` resets to a transparent background and `currentColor`.
- **Other**: `blockquote` gets a background and a start border. `figure`/`figcaption`, `sup`/`sub` and the `is-small`, `is-normal`, `is-medium` and `is-large` sizes are also covered.
- **Why we need it**: Bulma's minireset applies to every page.
  - It zeroes margin and padding on `p`, lists, headings, `pre` and similar elements.
  - It sets every heading to `font-size: 100%; font-weight: normal`, and `ul` to `list-style: none`.
  - So markdown rendered outside `.content` comes out flat. Our `.body`, `.comment-body` and glossary `.body` containers should also get `content`.

## Navbar

- **Minimal markup for two links**: put both links in `navbar-brand`. The brand "is always visible" on both touch and desktop widths ([navbar docs](https://bulma.io/documentation/components/navbar/)). Only `.navbar-menu` gets hidden: `display: none` by default, shown at 1024px and wider, and shown on touch only with `.is-active` (dist line 7544 and the `max-width: 1023px` block at line 7660).

  ```html
  <nav class="navbar" aria-label="main navigation">
    <div class="navbar-brand">
      <a class="navbar-item has-text-weight-bold" href="/" hx-get="/" hx-target="#main" hx-push-url="true">Loom</a>
      <a class="navbar-item" href="/sessions" hx-get="/sessions" hx-target="#main" hx-push-url="true">Sessions</a>
    </div>
  </nav>
  ```

- **The burger needs JavaScript.** Bulma ships none, and the docs give a vanilla JS snippet that toggles `is-active` on `.navbar-burger` and on the target `.navbar-menu`. With no burger and no `navbar-menu`, we need no JS.

## Class-collision audit

**Method**:

- **Bulma's side**: we took every `.class` token in a selector of the unminified `css/bulma.css` (1.0.4), after stripping comments, declaration blocks and quoted strings. That gives 3,290 class names. `bulma-no-dark-mode.css` gives 3,288.
- **Our side**:
  - Every `class="…"` in `internal/web/templates/*.html`, with template actions expanded to the values they can take:
    - `label-{{.Colour}}` gives `label-red` … `label-pink`.
    - `status-{{.Status}}` gives `status-closed`, `status-blocked`, `status-claimed` and `status-open`.
    - `column {{.Key}}` gives `frontier`, `claimed` and `blocked`.
    - The template also adds `selected`, `closed`, `active` and `zero`.
  - Every class selector in `internal/web/static/loom.css`.
  - The class selectors in `internal/systest` and `internal/web/*.go`, including test files.
  - `loom.js` uses no class names. The systest selectors add the `htmx-request`, `htmx-settling` and `htmx-swapping` classes.
- **The diff**: `comm -12` of the two lists. `.claude/worktrees` was ignored.

**Result: six collisions.** No `is-*` or `has-*` class of ours collides, and neither do the htmx classes.

In the table, "beats ours" means Bulma's `:not(:last-child)` rules have specificity (0,2,0), which outranks a single-class rule of ours even when `loom.css` loads after Bulma.

| Class | Where we use it | What Bulma does to it |
|---|---|---|
| **`title`** | `<span class="title">` in every list row and frontier row (`project.html`), and in board cards and decisions (`tabs.html`) | **Severe.** It is Bulma's page-heading element: `font-size: var(--bulma-title-size)` (size-3, 2rem), extrabold, `line-height: 1.125`, `color: --bulma-text-strong`, `word-break: break-word`. `.title:not(:last-child)` adds `margin-bottom: 1.5rem`, which beats ours, and that hits `.card .title`, which we make `display:block`. Every Issue title would render at 2rem extrabold. |
| **`label`** | `<span class="label label-{colour}">` and `<span class="type label">` (label chips everywhere) | Bulma's form-field label: `display: block`, `font-weight: semibold`, `font-size: --bulma-size-normal`, `color: --bulma-label-color`. If `loom.css` loads after Bulma, our `display`, `font-size` and colour win. The semibold weight still leaks, and `.label:not(:last-child) { margin-bottom: 0.5em }` beats ours, so chips other than the last get a bottom margin. If Bulma loads after `loom.css`, the chips turn into block-level form labels. |
| **`progress`** | `<p class="progress">` wrapping a `<progress>` element and its "N/M done" text (`home.html`) | Bulma's progress bar, which belongs on the `<progress>` element itself: `appearance: none`, `border: none`, `display: block`, `height: --bulma-size-normal` (1rem), `overflow: hidden`, `padding: 0`, `width: 100%`, a rounded radius and `margin-bottom` when not last. On our `<p>` this clips the 1.5-line-high text to 1rem and stretches the paragraph to full width. The bare `<progress>` element inside is left unstyled, because Bulma targets only `.progress`. |
| **`card`** | `<div class="card">` on the board (`tabs.html`) | Bulma's card component: `box-shadow: --bulma-card-shadow`, `border-radius: 0.75rem`, `background-color: --bulma-scheme-main`, `color: --bulma-card-color`, `max-width: 100%`, `position: relative`. Our radius, background and border win if `loom.css` loads later. The **shadow** leaks, and `.card:not(:last-child) { margin-bottom: 1.5rem }` beats our 0.4rem. This one is close to what we mean, so adopting Bulma's card is an option. |
| **`tabs`** | `<ul class="tabs">` on the Issue pane (`project.html`) | Bulma's tabs expect `div.tabs > ul > li > a`, and mark the active tab with `li.is-active`. Our `.tabs` is the `ul` itself and uses `li.active`, so the `.tabs ul` rules don't match and the active state is never styled. What does apply: `.tabs` gets `display: flex`, `justify-content: space-between` (leaks), `overflow: hidden; overflow-x: auto`, `white-space: nowrap`, `user-select: none` and `font-size`, and `.tabs:not(:last-child)` adds a `margin-bottom` that beats ours. `.tabs li` gets `display: block`. `.tabs a` gets a bottom border, `padding: 0.5em 1em`, `display: flex` and `margin-bottom: -1px`, plus hover border and colour changes. Our colour wins, but the border and padding leak. |
| **`column`** | `<section class="column {key}">` on the board (`tabs.html`); the systest selects `.column.claimed .card .session-link` | Bulma's grid column: `display: block`, `flex-basis: 0`, `flex-grow: 1`, `flex-shrink: 1`, `padding: var(--bulma-column-gap)` (0.75rem). Our `.board` is a CSS grid, so the flex properties have no effect, and our `padding: 0.5rem` wins if `loom.css` loads later. **Mostly harmless.** The other `.column` rules need a `.columns` parent or an `is-*` modifier. |

**Near-misses**: Bulma uses `is-active` and `is-selected` for states, and we use `active` and `selected`. These don't collide, but adopting Bulma's components means renaming them.

**Global element styles**: Bulma's base layer also restyles elements our CSS relies on:

- the reset described under `.content`
- `html` background and font size, `body` font family, colour and line height
- `a` colour (`--bulma-link-text`)
- `code` in red on grey, `pre`
- `table` with `border-collapse: collapse` and `th` colour
- `strong`, `small`, `hr`, `button` (`appearance: none; background: none; border: none`)

Our own rules for `body`, `a`, `code`/`pre` and `progress` will fight or duplicate these.
