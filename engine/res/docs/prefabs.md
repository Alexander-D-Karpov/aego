---
id: prefabs
icon: file.prefab
group: scenes
title: Prefabs
order: 1
---
# Prefabs

![A prefab instance selected in the Hierarchy: the orange cube marks instances](shots/prefab.png)

A prefab is a reusable piece of a scene stored in an `.aeprefab` file. Every copy in a scene is an instance: it shows the prefab and keeps its own changes, the overrides. When the prefab changes, every instance follows it, and its overrides stay.

## Create and place

![Prefab mode: the prefab opens on its own, with a breadcrumb back to the scene](shots/prefab-mode.png)

- Drag a node from the Hierarchy onto a folder in Assets, or use Hierarchy → Prefab → Create Prefab. The node becomes an instance of the new prefab and keeps its node ids, so references to it keep working.
- Drag an `.aeprefab` into the Hierarchy or the Scene View, or use Instantiate in Scene from its context menu.
- Open a prefab with a double click or Prefab → Open Prefab. Saving it updates every open instance in one step that can be undone from any of those scenes.

## Overrides

Any change to an instance is an override: a property, an added or removed component, an added or removed node, the order of children. The position, rotation and scale of the instance root are always the instance's own.

- The Hierarchy marks instance roots with the prefab icon and nodes you added with a plus.
- The Inspector marks overridden properties with a bar on the left and added components with an `added` badge. The Prefab card on top has Apply All, Revert All and Open.
- Right click a property, a component or a node to apply it to the prefab or revert it.

Nodes that come from a prefab keep their parent inside an instance. Unpack the instance to move them.

## Nested prefabs and variants

A prefab can contain instances of other prefabs. When you apply a change made inside a nested instance, choose where it goes: the nested prefab itself, or the outer prefab as an override of its nested instance.

A variant is a prefab whose root is an instance of another prefab: Prefab → Create Variant. It changes what it overrides and follows its base everywhere else.

## Unused overrides

When a prefab loses a node or a component that an instance overrides, the override is kept and listed under Unused overrides in the Prefab card. It does nothing until you remove it. Problems shows it as PFB002.

## Missing prefabs

If the prefab file is gone, the instance loads as an empty node named after it and is saved back unchanged. Problems shows PFB001. Play is not blocked.

## Unpack

- Unpack turns the instance into plain nodes; prefabs nested in it become instances of their own.
- Unpack Completely turns everything into plain nodes.

## Scripts

`scene.Instantiate(prefab, parent)` creates a copy at runtime. The copy has fresh node ids, and references inside it point into the copy.

`node.IsPrefabInstance()` reports whether a node is a prefab copy or lies inside one. `node.PrefabRoot()` returns the root of that copy.
