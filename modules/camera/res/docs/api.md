---
id: aego/camera.Pose.Position
---
Stores the camera's world-space centre in world units, not screen pixels.

---
id: aego/camera.Pose.Rotation
---
Stores the camera rotation in radians; blends use the shortest wrapped angular path.

---
id: aego/camera.Pose.Zoom
---
Stores the positive Camera2D zoom value. Increasing it magnifies the view and reduces the visible world area.

---
id: aego/camera
summary: Picks and blends virtual cameras, shakes the view and converts between screen and world.
---

Virtual cameras are components; this package switches between them and adds shake. Shake grows with trauma, which fades over time and is scaled by the accessibility setting. In the documentation sandbox nothing is drawn; the example logs how trauma fades. See [Cameras 2D](ref:camera).

## Example

```aes run
package demo

import (
    "aego/camera"
    "aego/log"
)

script Quake {
    @export var Strength float = 0.6

    var frames int

    func Ready() {
        camera.Shake(Strength)
        log.Info("trauma " + string(camera.Trauma()))
    }

    func Update(dt float) {
        frames++
        if frames % 20 == 0 {
            log.Info("trauma " + string(camera.Trauma()))
        }
        if frames == 60 {
            camera.Shake(0.3)
            log.Info("shaken again, trauma " + string(camera.Trauma()))
        }
    }
}
```
