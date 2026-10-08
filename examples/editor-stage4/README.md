# editor-stage4

Demo project for the Aego editor and its native extension API.

`native/module.go` is ordinary game code: the `Rotator` and `Health` components and the systems that drive them. It is compiled into both `aego run` and the editor.

`native/editor.go` is built only with `-tags aego_editor`, which `aego edit` adds automatically. It shows everything an extension can contribute:

- component metadata (`sdk.DescribeComponent`): Health and Rotator appear in the Inspector under Scripts
- a custom inspector with a Heal button below the generated Health fields
- a Quest panel (Window → Quest)
- a creation template: `Create → game → Enemy`. It lives under `game` because it comes from this project's module; it is not an engine node type
- a command, Heal All, bound to Ctrl+Shift+H

Run it with `aego edit examples/editor-stage4`. Editing anything in `native/` rebuilds the editor and restores your session.
