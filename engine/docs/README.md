# Documenting a module

The documentation site, the editor's documentation panel, hover text in the script editor and `docs_search` over MCP all read one registry, assembled from the engine and from every module. This page lists what a module provides and what `aego docs check --strict` holds it to. The writing rules are in `docs/style.md`, the terms in `docs/glossary.md`; the decision is D347 in ARCHITECTURE.md.

## What a module provides

- **Descriptions in Go.** `schema.Doc` on every component, `schema.FieldDoc` on every field the Inspector shows, `bind.Doc` on every script function, method and signal. A field without a description is an error even without `--strict`.
- **Pages.** Markdown files under `res/docs`, embedded and passed to `Registry.Docs`. Each file starts with a front matter block and may hold several pages separated by `---` blocks.
- **Try scenes.** `res/docs/try/<Component>.aescene`, with `<Component>.aes` for its script and `<Component>.<name>.aeprefab` for prefabs. The component page runs it in the browser.
- **Diagnostics.** `res/docs/diagnostics/<CODE>.md` for every code the module declares in `Info.Codes`, with an `example` scene that reproduces it.
- **Messages.** `res/i18n/en.aetext` and its translations, passed to `Registry.Messages`, with keys under `i18n.<module>.`. `go run ./tools/i18n extract` writes the English catalog from the `Tr` calls.

## Front matter

| Key | Meaning |
|---|---|
| `id` | Unique page id, used by `ref:` links. Required. |
| `title` | Title in the navigation and on the page. |
| `parent` | Id of the page this one sits under. |
| `group`, `group_title`, `group_icon`, `group_order` | The navigation group, or a new one. |
| `order` | Place among siblings. |
| `icon` | An editor atlas icon, such as `cat.gameplay`. |
| `summary` | One sentence for lists and search, at most 160 characters. Without it the first sentence of the opening paragraph is used. |
| `try`, `try_select` | A try scene for a guide page and the node it selects. |
| `keys` | Keywords whose hover text this page provides. |
| `search_names` | Search-only aliases, for example `[bloom, glow]`. Translated files can add local phrases; aliases work in both languages without changing script keyword hover keys. |
| `params` | Documentation-only names for every parameter of an existing function or method, for example `[key, value]`. Count and names are validated; bindings and ABI are unchanged. |
| `style_allow` | Lint patterns this page may match, such as `[agent]`. Say why in the review. |
| `editor_only` | The page is shown only in the editor. |
| `code`, `severity`, `scope`, `example` | Diagnostic pages only. |

## Writing

- Describe what the reader sees and does. Implementation details go to ARCHITECTURE.md.
- A description starts with the verb: "Pushes the body…", not "ApplyForce pushes the body…". The site removes a leading name, so Go comments may keep Go style.
- Give units and what 0 or -1 means: "Seconds of game time before the node comes back", "0 never snaps".
- One fact per sentence, at most four sentences per paragraph. A bold phrase that opens a paragraph is a heading; use a heading.
- Name things as the editor does: Rigid Body 2D in prose, `physics.Body2D` in code. Use the terms in `docs/glossary.md`.
- Every page opens with one or two sentences on what it covers, before the first heading.
- A diagnostic page says in one sentence what happened and in one how to fix it.

## Code blocks

| Fence | Checked |
|---|---|
| ` ```aes run ` | Compiled and shown with a Run button. |
| ` ```aes check ` | A whole file, compiled together with the `aes check` blocks above it on the page; a later version of a script replaces the earlier one. |
| ` ```aescene check ` | A whole scene, played for 60 frames with those scripts. A load error, a fault, a logged warning, a scene problem or a reference to a missing node fails the check. |
| ` ```go ` | Formatted with gofmt and built against `sdk`. |
| other | Shown as is. |

## Links

`[text](ref:<id>)` links a page, a component (`ref:gameplay.Motion2D`), a field or a script function. A broken `ref:` link is an error. A name in backticks that looks like a component, function or field must exist in the reference.

The website only auto-links exact qualified technical IDs and registered diagnostic codes in prose. It does not guess links for bare names such as `Floor`, `State` or `Range`, or for identifiers inside executable snippets: the syntax highlighter cannot resolve local declarations or shadowing. Add an explicit `ref:` link in the explanation when a snippet uses an API. Code copying preserves the original source.

## Checking

```sh
aego docs check --strict   # every rule, as CI runs it
aego docs report           # one row per page; a red row has errors
make site-check            # the check above, then the site build tests
```

The rule codes are DOC010 to DOC023; `aego docs check` prints each finding with its code, page and line. Prose patterns live in `engine/docs/lint.aetext`.
