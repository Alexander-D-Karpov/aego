---
id: render2d/sprites
parent: module/render2d
order: 1
title: Sprites
---
# Sprites

[SpriteRenderer](ref:engine.SpriteRenderer) draws a texture region at the node's 2D transform. Add it with Create → Sprite or Add Component → Rendering → Sprite Renderer.

## Size and pivot

[Size](ref:engine.SpriteRenderer.Size) is in world units. A zero size uses the texture size divided by the camera zoom, so a 64 px texture under a camera with zoom 64 is one unit wide. Create → Sprite sets the size to (1, 1).

[Origin](ref:engine.SpriteRenderer.Origin) is the pivot in normalized sprite space: (0.5, 0.5) is the center and the default, (0, 0) is the top-left corner. The node rotates and scales around it.

## Draw order

Sprites are sorted by [Layer](ref:engine.SpriteRenderer.Layer) and then by [Order](ref:engine.SpriteRenderer.Order) inside the layer. Higher values are drawn on top.

## Color and blending

[Tint](ref:engine.SpriteRenderer.Tint) is multiplied with the texture; its alpha controls transparency. A sprite without a texture draws a solid rectangle in the tint color. [Blend](ref:engine.SpriteRenderer.Blend) selects premultiplied alpha, opaque or additive blending, and [Filter](ref:engine.SpriteRenderer.Filter) selects nearest sampling for pixel art or linear sampling for smooth art.

## From scripts

```aes
sr := node.Get<SpriteRenderer>()
if sr != nil {
    sr.Tint = Color{1, 0.4, 0.4, 1}
    sr.FlipX = true
}
```
