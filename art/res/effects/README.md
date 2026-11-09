# Built-in effect drawings

These eight editable 32×32, eight-frame `.aeart` documents are the shared
native/web Asset Studio starters. They contain ordinary raster layers, named
palette colors, a playback tag, a pivot, and emission cels where appropriate.
`aego.effectTemplate` identifies the matching existing particle preset contract.
The files do not contain scripts, external resources, or a particle simulator.

The initial pixel artwork is original Aego sample data under the repository
license. It is generated using the **existing Go Art Core drawing operations**
and normal serializer. No separate pixel implementation is used by either UI.

To deliberately regenerate the checked-in files from their source artwork:

```sh
AEGO_GENERATE_EFFECT_TEMPLATES=1 go test -tags aego_generate_effects ./art \
  -run '^TestGenerateEffectTemplates$' -count=1
```

The generator is excluded from normal builds and tests. Its stable starter
document IDs make the output reproducible. Creating a new user document gives
it a fresh document identity; the embedded source bytes are never edited.
