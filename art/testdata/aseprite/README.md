# Real Aseprite test fixtures

The `.aseprite` files under `upstream/` are unmodified test sprites from
[Aseprite's test suite](https://github.com/aseprite/aseprite/tree/3669f4774d181d77deb3e6db95fe6df8ca347410/tests/sprites),
commit `3669f4774d181d77deb3e6db95fe6df8ca347410`. The upstream
[`tests/LICENSE.txt`](https://github.com/aseprite/aseprite/blob/3669f4774d181d77deb3e6db95fe6df8ca347410/tests/LICENSE.txt)
is reproduced alongside these fixtures. This does not assert that the Aseprite
application itself is MIT licensed.

The `reference/` PNGs were independently rendered from those files using the
Linux x86-64 LibreSprite **1.3** release AppImage (SHA-256
`fd90bddda8135fe00f4a9a9c8c9ebd48f9b07e8782ab269be5c9efb1a851af30`).
For each supported file, in batch mode with audio disabled:

```sh
libresprite -b upstream/NAME.aseprite --save-as reference/NAME/frame.png
```

LibreSprite adds 1-based frame numbers for multi-frame images. These PNGs are
derived from the MIT-licensed sprites, not from the Aego importer. Tests compare
all visible RGBA pixels, frame count, actual metadata, linked-image topology,
and source round-trip. RGB values in fully transparent pixels are deliberately
not compared: the `bg-index-3` reference contains nonzero hidden RGB there,
while Aego's composed output normalizes that invisible data. This is not a
claim of a lossless import for all Aseprite files.

`2x2tilemap2x2tile.aseprite` exercises the explicitly unsupported tileset
chunk and has no rendered reference. `slices.aseprite` exercises the rejected
indexed background case, while `tags3x123reps.aseprite` and
`file-tests-props.aseprite` exercise warnings about discarded metadata and
finite tag repeat counts. ICC/custom-gamma profiles, animated palettes,
tilemaps, nonzero cel z-index, and other unsupported features must continue
to fail or report diagnostics as documented by `ImportAseprite`.
