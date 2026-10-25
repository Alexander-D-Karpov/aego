---
id: agents
icon: panel.console
title: Working with AI agents
group: tools
order: 3
---

# Working with AI agents

`aego mcp` serves a project to an AI agent over the Model Context Protocol. The agent edits scenes, prefabs and data assets through the same operations the editor uses, reads the documentation and the type schema, and runs scenes headless to check its work. Every tool is also a command of `aego`, so anything an agent does can be repeated by hand or in CI.

## Connecting

Claude Code, from the project directory:

    claude mcp add aego -- aego mcp --project . --watch

or a `.mcp.json` next to `project.aego`:

    {"mcpServers": {"aego": {"command": "aego", "args": ["mcp", "--watch"]}}}

Other clients take the same command. The server speaks MCP over stdio; logs go to stderr. A project with native code is served by its own binary, so its components are known.

## Tools

Each tool has the name of its command with underscores (`scene set` is `scene_set`) and the same arguments; `aego scene help` lists them, and the CLI reference has the full table. `docs_search` and `docs_read` read the documentation. Write tools take `dry-run`, which reports the changes and writes nothing. The script tools answer with what the compiler says about the new text, the same `AES` errors as `aego check`. In the editor the agent also has `ask_user`, which puts a question with options in the Agent panel and waits for the answer.

## Resources

| URI | Content |
|---|---|
| `aego://project/manifest` | `project.aego` |
| `aego://project/problems` | what `project_validate` reports |
| `aego://docs/index`, `aego://docs/<id>` | documentation |
| `aego://schema/index`, `aego://schema/<type>` | types and their fields |
| `aego://file/<path>` | a scene, prefab, data asset or table as text |

With `--watch` the server polls the project and sends `resources/updated` for subscribed resources when files change, including changes made in the editor.

## A typical session

1. `scene_tree` and `schema_show` to see what is there and what a component takes.
2. Changes with `scene_set`, `scene_add_node`, `scene_add_comp`, `prefab_instantiate`, or several at once with `scene_apply`; `dry-run` first when unsure.
3. `scene_validate`, then `scene_run` with a number of frames: the log has the warnings and errors, and the state hash shows whether two runs behave the same.

## With the editor open

The editor and the agent can work on the same project. A scene, prefab or data asset changed on disk reloads in the editor when it has no unsaved changes; with unsaved changes the editor asks whether to reload or keep its version, and data assets warn and keep yours until you save or revert. Prefab instances in open scenes follow a changed prefab file. While the editor is open, tools that create assets refuse: create them in the editor, or close it.
