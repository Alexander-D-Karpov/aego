// Linux/Xvfb only: a private session leader anchors the process-group ID through browser shutdown, preventing signals to a reused ID or the user's browsers.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";

const self = fileURLToPath(import.meta.url);

function proc(pid) {
    let value;
    try { value = fs.readFileSync(`/proc/${pid}/stat`, "utf8"); }
    catch (e) { if (e.code === "ENOENT" || e.code === "ESRCH") return null; throw e; }
    const close = value.lastIndexOf(")");
    if (close < 0) throw new Error(`invalid process record ${pid}`);
    const fields = value.slice(close + 2).trim().split(/\s+/);
    if (fields.length < 20) throw new Error(`short process record ${pid}`);
    return { pid: Number(pid), state: fields[0], group: Number(fields[2]), session: Number(fields[3]), start: fields[19] };
}

function sessionMembers(session) {
    const members = [];
    for (const entry of fs.readdirSync("/proc")) {
        if (!/^\d+$/.test(entry)) continue;
        const p = proc(entry);
        if (p && p.session === session && !["Z", "X", "x"].includes(p.state)) members.push(p);
    }
    return members;
}

function ownedMembers(session, token) {
    const found = new Map(sessionMembers(session).map(p => [p.pid, p]));
    const marker = `AEGO_VISIBILITY_OWNER=${token}`;
    // Chromium helpers may escape the session: detect the inherited random marker, never kill by name, and fail closed with the profile retained if a helper outlives the browser.
    for (const entry of fs.readdirSync("/proc")) {
        if (!/^\d+$/.test(entry) || found.has(Number(entry))) continue;
        try {
            if (fs.statSync(`/proc/${entry}`).uid !== process.getuid()) continue;
            const vars = fs.readFileSync(`/proc/${entry}/environ`, "utf8").split("\0");
            if (!vars.includes(marker)) continue;
            const p = proc(entry);
            if (p && !["Z", "X", "x"].includes(p.state)) found.set(p.pid, p);
        } catch (e) {
            if (!["ENOENT", "ESRCH", "EACCES", "EPERM"].includes(e.code)) throw e;
        }
    }
    return [...found.values()];
}

export function launchVisibilityBrowser(executable, args, stderr) {
    const token = randomUUID();
    const guard = spawn(process.execPath, [self, "--guardian"], {
        detached: true, stdio: ["ignore", "ignore", stderr, "ipc"],
        env: { ...process.env, AEGO_VISIBILITY_OWNER: token },
    });
    const handle = { guard, token, started: false, exited: false, error: null, identity: null };
    guard.on("error", e => { handle.error = e; });
    guard.on("message", m => {
        if (m?.type === "started") handle.started = true;
        else if (m?.type === "exit") handle.exited = true;
        else if (m?.type === "error") handle.error = new Error(m.message);
    });
    guard.on("spawn", () => {
        try {
            handle.identity = proc(guard.pid);
            if (!handle.identity || handle.identity.group !== guard.pid || handle.identity.session !== guard.pid)
                throw new Error("browser guardian has no private session");
            guard.send({ type: "start", executable, args });
        } catch (e) { handle.error = e; }
    });
    return handle;
}

function checkGuardian(handle) {
    const p = proc(handle.guard.pid);
    if (!p || !handle.identity || p.start !== handle.identity.start ||
        p.group !== p.pid || p.session !== p.pid)
        throw new Error("lost private browser guardian; refusing to signal other processes");
    return p.pid;
}

async function until(test, timeout, message) {
    const end = Date.now() + timeout;
    do { if (test()) return; await sleep(50); } while (Date.now() < end);
    const error = new Error(message);
    error.code = "VISIBILITY_TIMEOUT";
    throw error;
}

export async function waitForVisibilityBrowser(handle) {
    await until(() => {
        if (handle.error) throw handle.error;
        if (handle.guard.exitCode !== null || handle.guard.signalCode) throw new Error("browser guardian exited at launch");
        return handle.started;
    }, 10000, "browser launch did not finish");
}

export async function stopVisibilityBrowser(handle, { graceMs = 5000, killMs = 5000 } = {}) {
    const pid = checkGuardian(handle);
    // The guardian catches SIGTERM; its browser processes do not inherit that
    // handler across exec. It remains alive while they release the profile.
    process.kill(-pid, "SIGTERM");
    let members;
    const noChildren = () => {
        checkGuardian(handle);
        members = ownedMembers(pid, handle.token).filter(p => p.pid !== pid);
        return members.length === 0;
    };
    try { await until(noChildren, graceMs, "browser descendants did not exit after SIGTERM"); }
    catch (e) {
        if (e.code !== "VISIBILITY_TIMEOUT") throw e;
        checkGuardian(handle);
        members = ownedMembers(pid, handle.token).filter(p => p.pid !== pid);
        if (members.some(p => p.group !== pid))
            throw new Error("browser created another process group; profile retained and no unsafe signal sent");
        // SIGKILL necessarily ends the guardian as well. The group is anchored
        // by it until this exact signal is sent; never signal this ID again.
        process.kill(-pid, "SIGKILL");
        await until(() => !ownedMembers(pid, handle.token).length, killMs, "browser process group survived SIGKILL");
        await until(() => handle.guard.exitCode !== null || handle.guard.signalCode, killMs, "browser guardian did not exit");
        return;
    }
    if (!handle.guard.connected) throw new Error("browser guardian IPC closed before cleanup");
    handle.guard.send({ type: "finish" });
    await until(() => handle.guard.exitCode !== null || handle.guard.signalCode, killMs, "browser guardian did not finish");
    if (handle.guard.exitCode !== 0) throw new Error("browser guardian exited abnormally during cleanup");
}

// Retry only temporary filesystem races, never ignore the final error.
export async function removeVisibilityProfile(dir, { remove = fs.promises.rm, retries = 5, delayMs = 100 } = {}) {
    const info = fs.lstatSync(dir);
    if (!info.isDirectory() || info.isSymbolicLink() || !path.basename(dir).startsWith("aego-visible-"))
        throw new Error("refusing to remove an unexpected browser profile");
    for (let attempt = 0; ; attempt++) {
        try { await remove(dir, { recursive: true, force: false }); return; }
        catch (e) {
            if (attempt >= retries || !["ENOTEMPTY", "EBUSY", "EPERM"].includes(e.code)) throw e;
            await sleep(delayMs * (attempt + 1));
        }
    }
}

if (process.argv[1] === self && process.argv[2] === "--guardian") {
    // This session leader is deliberately kept alive even after browser exit.
    process.on("SIGTERM", () => {});
    let browser;
    const send = message => { if (process.connected) process.send(message); };
    process.on("message", m => {
        if (m?.type === "start" && !browser) {
            browser = spawn(m.executable, m.args, { stdio: ["ignore", "ignore", "inherit"] });
            browser.on("error", e => send({ type: "error", message: String(e) }));
            browser.on("spawn", () => send({ type: "started" }));
            browser.on("exit", () => send({ type: "exit" }));
        } else if (m?.type === "finish") process.exit(0);
    });
    process.on("disconnect", () => {
        // If the test runner crashes, terminate only this owned group.
        process.kill(-process.pid, "SIGTERM");
        setTimeout(() => process.kill(-process.pid, "SIGKILL"), 5000);
    });
}
