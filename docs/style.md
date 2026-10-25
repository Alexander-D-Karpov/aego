# Writing the Aego documentation

The site is read by people making games, not by people building the engine. Write what they see and what they do.

## Open every page the same way

- The first paragraph says what the page is about and when the reader needs it, in two to four sentences.
- Do not start with "In this guide we…". Start with the subject.

## Guides and references

- A guide explains how and why: it walks through a task.
- A reference lists: every field, function, flag or code, each with one short description.

## Say only what the reader needs

- Describe behaviour the reader can see or change. Leave out how it is implemented: buffer sizes, algorithms, internal packages and functions, decision numbers, log formats, release mechanics. Those belong in ARCHITECTURE.md or docs/contributing.md.
- Give a number only when the reader sets it or runs into it: a default they can change, a limit they can hit.
- Say a thing once. Link to the page that owns it instead of repeating it. A component page owns its fields; a guide links there.
- No filler: simply, just, easily, basically, of course, note that, please, in order to, it is important to.
- No history. Past tense and the history of decisions belong in ARCHITECTURE.md. Never write "we added", "in this stage", "this commit" or "the agent" about the work on Aego.

## Keep it short

- One fact per sentence. Split sentences joined by semicolons.
- A paragraph has at most four sentences. More than that is a list, a table or a new section.
- Steps the reader follows are a numbered list, one action per step.
- A bold phrase at the start of a paragraph is a heading. Use a heading.
- Start a page with one or two sentences: what it is and when to use it.

## Examples

- One idea, one example.
- Every code block is checked: `aes` blocks compile, `go` blocks are whole files that build against `sdk`.

## Name things the same way

- Address the reader as "you". Headings are nouns: "Rewinding", not "How to rewind".

- Use the terms in docs/glossary.md and the names the editor shows: Rigid Body 2D in prose, `physics.Body2D` in code.
- Menu paths in bold with arrows: **Window → Theme**. Keys in backticks: `Ctrl+Shift+A`.
- Field, method and event descriptions start with the verb, not the name: "Pushes the body…", not "ApplyForce pushes the body…". The site strips a leading name from Go doc comments, so code comments can keep Go style.
- Fields that only have values during Play are listed as runtime state; their description does not say "during Play".

## Write for translation

- Plain sentences without idioms, jokes or wordplay.
- One term for one thing across all pages.
- No sentences built from fragments that depend on English word order, such as "Click X to Y-ify it".

## Page types

| Type | Shape |
|---|---|
| Guide | what and when; steps; a table of options; See also |
| Concept | what it is; how it behaves; what the reader controls; See also |
| Component page | generated: summary, Usage, Fields, Runtime state, Methods, Signals, Script events, Related guides |
| Diagnostic | one sentence on what happened, then **Fix.** and one sentence on how to fix it, then an example that reproduces it |

## Before and after

Before: "Current velocity in units per second, during Play." with the flag "read-only during Play only".
After: "Velocity in units per second." under Runtime state.

Before: "OnCollisionExit runs when the bodies stop touching, including when one of them is destroyed or disabled. Contact.Point is the last known point."
After: "Runs when the bodies stop touching, also when one is destroyed or disabled. Contact.Point is the last point of contact."

Before: "How often depends on how heavy the game is: often enough that rewinding replays about 1.5 ms of game, but no more than every 30 ticks and no less than every 300."
After: "The editor keeps copies of the game along the timeline, so rewinding is quick even in long sessions."

## Checks

`aego docs check` reports these rules as warnings; `--strict` makes them errors. `aego docs report` lists every page with its findings, and a red row is a page with errors in strict mode.

| Code | Rule |
|---|---|
| DOC010 | Prose, outside code, matches a pattern of engine/docs/lint.aetext |
| DOC011 | A decision number (D123) or an internal package path in prose |
| DOC012 | A paragraph longer than 120 words |
| DOC013 | A paragraph that starts with a bold phrase ending in a period or colon |
| DOC014 | A field description longer than 160 characters or two sentences |
| DOC015 | A runtime field description that says "during Play" |
| DOC016 | A method, signal or event whose first sentence is longer than 160 characters |
| DOC017 | A sentence of 12 or more words that appears on two pages |
| DOC019 | A guide page without an opening paragraph before its first section |
| DOC020 | A name in backticks that looks like `Type.Member`, `pkg.Name` or `Call()` but names nothing in the reference |
| DOC021 | A component field without a description; always an error |
| DOC022 | A `go` block that gofmt cannot parse, that is a fragment, or that does not build against `sdk` |
| DOC023 | A diagnostic page whose "what" or "fix" part is not exactly one sentence |

A link to a page that does not exist (`ref:` to an unknown id) is always an error.

engine/docs/lint.aetext holds the DOC010 patterns, one `pattern` block each with `regex` (a Go regular expression, matched without case), `why` and an optional `skip_group`, a docs group where the pattern is allowed. It is embedded in `aego`, so an installed copy checks a project without the engine source. A page that needs a matched word in its own sense, such as "just pressed" on the input page, lists it in its front matter: `style_allow: [just]`.
