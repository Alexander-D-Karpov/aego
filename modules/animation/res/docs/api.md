---
id: aego/anim
summary: Names and ids of animation events, for OnAnimEvent and the Event signal of an Animation Player.
---

An animation clip reaches an event, and the scripts on the player's node get `OnAnimEvent(id int, payload float)`. Events are declared by name in Project Settings → Animation; `anim.EventName` and `anim.EventID` turn ids into names and back. An unknown id has the name `""`, an unknown name the id `-1`. See [Animation](ref:animation).

## Example

```aes run
package demo

import (
	"aego/anim"
	"aego/log"
)

script Listener {
	var hit int

	func Ready() {
		hit = anim.EventID("Hit")
		log.Info("Hit has the id " + string(hit) + ", id 0 is called '" + anim.EventName(0) + "'")
	}

	func OnAnimEvent(id int, payload float) {
		if id == hit {
			log.Info("hit for " + string(payload))
		}
	}
}
```
