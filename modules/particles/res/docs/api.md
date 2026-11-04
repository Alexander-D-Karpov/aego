---
id: aego/particles
summary: Enable or disable decorative particles for the current Play world.
---

Use the session setting to compare the same route with and without the effects.

## Example

```aes check
package demo

import "aego/particles"

script Effects {
    func Ready() {
        particles.SetEnabled(true)
    }
}
```
