---
id: tools
icon: panel.console
title: Command line and agents
group: tools
order: 2
---

# Command line and agents

Everything the editor does to a project is also available from the `aego` command, so checks, tests, builds and documentation run the same way on a laptop, in CI and for an AI agent working on the project.

## Everyday commands

| Command | What it does |
|---|---|
| `aego new <dir> --template 2d` | creates a project from a template and checks it |
| `aego edit [path]` | opens the project at path, or the editor on a scene or prefab file, or the launcher when no project is found; `--launcher new` opens the launcher |
| `aego run [dir]` | runs the main scene and reloads assets and scripts on save |
| `aego check [dir]` | compiles every script package and checks the project; script errors exit 1, broken native code exits 2 |
| `aego test [dir]` | runs `*_test.aes` script tests; `--determinism` runs the main scene twice and compares state hashes |
| `aego export [dir]` | builds a distributable game for a target |
| `aego docs serve` | serves this documentation, with your project's documented scripts, on localhost |

## In CI

`aego check` and `aego test` exit with a non-zero status on any error, print every diagnostic with its file, line and code, and need no window or GPU. A typical pipeline runs both on every push and `aego export --check` before a release.

## For agents

- `aego docs export --out docs.json` writes the whole documentation registry as JSON: guides, every script function, component, field and diagnostic, and the doc comments of your own scripts. Give an agent this file instead of the website.
- The site publishes `llms.txt`, a plain list of every page with its group and summary.
- Diagnostics have stable codes such as `AES2050`; the Diagnostics reference explains each one and how to fix it.
