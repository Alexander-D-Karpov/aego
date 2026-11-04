import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { setTimeout as sleep } from "node:timers/promises";
import { launchVisibilityBrowser, waitForVisibilityBrowser, stopVisibilityBrowser, removeVisibilityProfile } from "./visibility-browser.mjs";
import { BrowserEventTimeout, visibilityFailureOutcome } from "./ci-warning-policy.mjs";

const profile = () => fs.mkdtempSync(path.join(os.tmpdir(), "aego-visible-"));
const done = async (test, message) => {
    const deadline = Date.now() + 5000;
    while (!test()) { if (Date.now() > deadline) throw new Error(message); await sleep(20); }
};

test("waits for late-writing browser descendants without touching another process", async () => {
    const dir = profile();
    const stranger = spawn(process.execPath, ["-e", "setInterval(()=>{},1000)"], {stdio:"ignore"});
    const descendant = `const fs=require('fs'); const d=process.argv[1];
        process.on('SIGTERM',()=>setTimeout(()=>{fs.writeFileSync(d+'/late','done');process.exit(0)},150));
        fs.writeFileSync(d+'/ready','yes'); setInterval(()=>{},1000);`;
    const fake = `const {spawn}=require('child_process'); spawn(process.execPath,['-e',process.argv[1],process.argv[2]],{stdio:'ignore'});
        setInterval(()=>{},1000);`;
    let browser;
    try {
        browser = launchVisibilityBrowser(process.execPath, ["-e", fake, descendant, dir], "ignore");
        await waitForVisibilityBrowser(browser);
        await done(() => fs.existsSync(path.join(dir, "ready")), "fixture child did not start");
        await stopVisibilityBrowser(browser);
        browser = null;
        assert.equal(fs.readFileSync(path.join(dir, "late"), "utf8"), "done");
        assert.equal(stranger.exitCode, null);
        assert.equal(stranger.signalCode, null);
        await removeVisibilityProfile(dir);
        assert.equal(fs.existsSync(dir), false);
    } finally {
        if (browser) await stopVisibilityBrowser(browser);
        stranger.kill("SIGTERM");
        if (fs.existsSync(dir)) await removeVisibilityProfile(dir);
    }
});

test("escalates only the private group if a child ignores SIGTERM", async () => {
    const dir = profile();
    const code = `require('fs').writeFileSync(process.argv[1]+'/ready','yes');process.on('SIGTERM',()=>{});setInterval(()=>{},1000)`;
    let browser;
    try {
        browser = launchVisibilityBrowser(process.execPath, ["-e", code, dir], "ignore");
        await waitForVisibilityBrowser(browser);
        await done(() => fs.existsSync(path.join(dir,"ready")), "fixture did not start");
        await stopVisibilityBrowser(browser, {graceMs: 100, killMs: 5000});
        browser = null;
        await removeVisibilityProfile(dir);
        assert.equal(fs.existsSync(dir), false);
    } finally {
        if (browser) await stopVisibilityBrowser(browser);
        if (fs.existsSync(dir)) await removeVisibilityProfile(dir);
    }
});

test("retains the profile and refuses to signal a helper in a different session", async () => {
    const dir = profile();
    const child = `require('fs').writeFileSync(process.argv[1]+'/escaped','started');setTimeout(()=>process.exit(0),900)`;
    const parent = `const {spawn}=require('child_process');spawn(process.execPath,['-e',process.argv[1],process.argv[2]],{stdio:'ignore',detached:true}).unref();setInterval(()=>{},1000)`;
    let browser;
    try {
        browser = launchVisibilityBrowser(process.execPath, ["-e", parent, child, dir], "ignore");
        await waitForVisibilityBrowser(browser);
        await done(() => fs.existsSync(path.join(dir,"escaped")), "detached fixture did not start");
        await assert.rejects(stopVisibilityBrowser(browser, {graceMs:100}), /another process group/);
        assert.equal(fs.readFileSync(path.join(dir,"escaped"),"utf8"), "started");
        await sleep(1000);
        await stopVisibilityBrowser(browser);
        browser = null;
        await removeVisibilityProfile(dir);
    } finally {
        if (browser) { await sleep(1000); await stopVisibilityBrowser(browser); }
        if (fs.existsSync(dir)) await removeVisibilityProfile(dir);
    }
});

test("retries temporary ENOTEMPTY but does not hide a persistent cleanup error", async () => {
    const dir = profile();
    let attempts = 0;
    const locked = () => { const e = new Error("injected cleanup failure"); e.code = "ENOTEMPTY"; return e; };
    try {
        await assert.rejects(removeVisibilityProfile(dir, {retries:2, delayMs:1, remove:async()=>{ attempts++; throw locked(); }}), /injected cleanup failure/);
        assert.equal(attempts, 3);
        assert.equal(fs.existsSync(dir), true);
        attempts = 0;
        await removeVisibilityProfile(dir, {retries:2, delayMs:1, remove:async(p, opts)=>{
            attempts++; if (attempts < 3) throw locked(); await fs.promises.rm(p,opts);
        }});
        assert.equal(attempts, 3);
        assert.equal(fs.existsSync(dir), false);
    } finally { if (fs.existsSync(dir)) await removeVisibilityProfile(dir); }
});

test("a cleanup error cannot become a Firefox timeout warning", () => {
    const setup = new BrowserEventTimeout("ready");
    const cleanup = new Error("profile still in use");
    assert.equal(visibilityFailureOutcome("firefox", setup, true, false), "warning");
    assert.equal(visibilityFailureOutcome("firefox", new AggregateError([setup, cleanup], "cleanup failed"), true, false), "fail");
});

test("browser launch failures remain errors and the private guardian can be cleaned up", async () => {
    const dir = profile();
    let browser;
    try {
        browser = launchVisibilityBrowser(path.join(dir, "missing-browser"), [], "ignore");
        await assert.rejects(waitForVisibilityBrowser(browser), /ENOENT/);
        await stopVisibilityBrowser(browser);
        browser = null;
        await removeVisibilityProfile(dir);
    } finally {
        if (browser) await stopVisibilityBrowser(browser);
        if (fs.existsSync(dir)) await removeVisibilityProfile(dir);
    }
});
