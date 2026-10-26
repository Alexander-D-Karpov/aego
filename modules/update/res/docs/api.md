---
id: aego/update
summary: Finds a newer version of the game on its update server, downloads it and restarts the game on it.
---

A game updates itself when `project.aego` names its update server and the key that signs its releases:

```
update {
  url="https://example.com/mygame/updates"
  publicKey="<the public key from aego release keygen>"
}
```

The server holds one folder per channel, `stable/release.json` and `nightly/release.json`, each with its `release.json.sig`. `aego export --release-json` writes both next to the exported archive. The game checks at start; in the Auto mode it downloads the new version in the background, and the Settings View gets **Updates** and **Update channel**. Nothing installs without a valid signature and checksum. The sandbox has no update server, so this example needs a project.

## Example

```aes check
package demo

import (
    "aego/log"
    "aego/update"
)

script UpdateBanner {
    var shown bool

    func Update(dt float) {
        if !shown && update.State() == "ready" {
            shown = true
            log.Info("version " + update.Version() + " is ready: " + update.Notes())
        }
    }

    func Restart() {
        if update.State() == "ready" {
            update.Apply()
        }
    }
}
```
