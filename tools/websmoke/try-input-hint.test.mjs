import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";

class Element {
    constructor() {
        this.childNodes = [];
        this.listeners = new Map();
        this.dataset = {};
        this.className = "";
        this.classList = { add() {}, remove() {}, toggle() {} };
        this.style = {};
        this.parent = null;
    }
    set textContent(value) {
        this.text = value;
        if (value === "") {
            for (const child of this.childNodes) child.parent = null;
            this.childNodes = [];
        }
    }
    get textContent() { return this.text || ""; }
    set innerHTML(_value) {
        this.progress = new Map([[".bar i", new Element()], [".phase", new Element()], [".label", new Element()]]);
    }
    appendChild(child) {
        child.remove();
        child.parent = this;
        this.childNodes.push(child);
        return child;
    }
    append(...children) { for (const child of children) this.appendChild(child); }
    remove() {
        if (this.parent) this.parent.childNodes = this.parent.childNodes.filter(child => child !== this);
        this.parent = null;
    }
    querySelector(selector) {
        if (this.progress?.has(selector)) return this.progress.get(selector);
        if (selector === ".try-input") return this.childNodes.find(child => child.className === "try-input") || null;
        return null;
    }
    querySelectorAll() { return []; }
    addEventListener(name, fn) {
        if (!this.listeners.has(name)) this.listeners.set(name, new Set());
        this.listeners.get(name).add(fn);
    }
    removeEventListener(name, fn) { this.listeners.get(name)?.delete(fn); }
    emit(name, event = {}) { for (const fn of [...(this.listeners.get(name) || [])]) fn(event); }
    setAttribute() {}
    getBoundingClientRect() { return { left: 100, top: 50, width: 600, height: 400 }; }
}

class Block extends Element {
    constructor() {
        super();
        this.dataset = { scene: "scene", input: "Space" };
        this.stage = new Element();
        this.stage.closest = () => this;
        this.buttons = new Map();
        for (const name of [".try-start", ".try-reset", ".try-download", ".try-copy", ".try-status"])
            this.buttons.set(name, new Element());
        this.stage.appendChild(this.buttons.get(".try-start"));
    }
    querySelector(selector) {
        if (selector === ".try-stage") return this.stage;
        return this.buttons.get(selector) || null;
    }
}

function fixture() {
    const blocks = [new Block(), new Block()];
    let canvas;
    let viewport = { x: 0, y: 28, width: 320, height: 372, canvasWidth: 600, canvasHeight: 400 };
    const api = { call(method) {
        if (method === "viewport") return JSON.stringify({ viewport });
        if (method === "load") return JSON.stringify({ text: "scene" });
        return "{}";
    } };
    const document = {
        documentElement: { lang: "en" },
        querySelectorAll: () => blocks,
        addEventListener() {},
        createElement(tag) {
            const element = new Element();
            if (tag === "canvas") canvas = element;
            return element;
        },
    };
    const context = { document, Number, JSON, Map, Promise, setTimeout, clearInterval, setInterval,
        aegoRuntime: () => Promise.resolve(api), aegoRuntimeCached: () => Promise.resolve(false) };
    context.globalThis = context;
    vm.runInNewContext(readFileSync(new URL("../gendocs/site/static/try.js", import.meta.url), "utf8"), context);
    return { blocks, get canvas() { return canvas; }, setViewport(v) { viewport = v; } };
}

async function start(f, index = 0) {
    f.blocks[index].buttons.get(".try-start").emit("click");
    await new Promise(resolve => setImmediate(resolve));
    assert.ok(f.blocks[index].stage.querySelector(".try-input"));
}

test("focus and Inspector, toolbar, secondary or non-primary clicks retain the input hint", async () => {
    const f = fixture();
    await start(f);
    const hint = () => f.blocks[0].stage.querySelector(".try-input");
    f.canvas.emit("focus");
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 500, clientY: 150 });
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 60 });
    f.canvas.emit("pointerdown", { button: 2, isPrimary: true, clientX: 200, clientY: 150 });
    f.canvas.emit("pointerdown", { button: 0, isPrimary: false, clientX: 200, clientY: 150 });
    assert.ok(hint());
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 150 });
    assert.equal(hint(), null);
    assert.equal(f.canvas.listeners.get("pointerdown").size, 0);
});

test("invalid viewport is ignored and CSS scaling maps a game click correctly", async () => {
    const f = fixture();
    await start(f);
    f.setViewport({ x: 0, y: 20, width: 0, height: 500, canvasWidth: 1200, canvasHeight: 800 });
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 150 });
    assert.ok(f.blocks[0].stage.querySelector(".try-input"));
    f.setViewport({ x: 0, y: 40, width: 600, height: 500, canvasWidth: 1200, canvasHeight: 800 });
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 450, clientY: 150 });
    assert.ok(f.blocks[0].stage.querySelector(".try-input"));
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 150 });
    assert.equal(f.blocks[0].stage.querySelector(".try-input"), null);
});

test("stacked mobile Inspector does not dismiss the Game View hint", async () => {
    const f = fixture();
    await start(f);
    f.setViewport({ x: 0, y: 28, width: 600, height: 210, canvasWidth: 600, canvasHeight: 400 });
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 350 });
    assert.ok(f.blocks[0].stage.querySelector(".try-input"));
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 120 });
    assert.equal(f.blocks[0].stage.querySelector(".try-input"), null);
});

test("switching examples removes the old canvas listener", async () => {
    const f = fixture();
    await start(f, 0);
    assert.equal(f.canvas.listeners.get("pointerdown").size, 1);
    await start(f, 1);
    assert.equal(f.blocks[0].stage.querySelector(".try-input"), null);
    assert.equal(f.canvas.listeners.get("pointerdown").size, 1);
    f.canvas.emit("pointerdown", { button: 0, isPrimary: true, clientX: 200, clientY: 150 });
    assert.equal(f.blocks[1].stage.querySelector(".try-input"), null);
    assert.equal(f.canvas.listeners.get("pointerdown").size, 0);
});
