import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const wrapper = fileURLToPath(new URL("./private-xvfb.sh", import.meta.url));

test("WebGL runner uses a private display and forwards arguments and exit status", (t) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "aego-xvfb-test-"));
    t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
    const xvfb = path.join(dir, "xvfb-run");
    fs.writeFileSync(xvfb, `#!/bin/sh
printf '%s\\n' "$1" "$2" "$3" > "$AEGO_TEST_XVFB_ARGS"
shift 3
DISPLAY=:private-test
export DISPLAY
exec "$@"
`, {mode:0o755});
    const output = path.join(dir, "args");
    const js = `process.stdout.write(JSON.stringify({display:process.env.DISPLAY, private:process.env.AEGO_PRIVATE_XVFB, arg:process.argv[1]})); process.exit(37)`;
    const run = spawnSync("/bin/sh", [wrapper, process.execPath, "-e", js, "a b;$HOME"], {
        encoding:"utf8", env:{...process.env, PATH:`${dir}:/usr/bin:/bin`, DISPLAY:":user-desktop", AEGO_TEST_XVFB_ARGS:output},
    });
    assert.equal(run.status, 37, run.stderr);
    assert.deepEqual(JSON.parse(run.stdout), {display:":private-test", private:"1", arg:"a b;$HOME"});
    assert.deepEqual(fs.readFileSync(output, "utf8").trimEnd().split("\n"),
        ["-a", "-s", "-screen 0 1600x1000x24 -nolisten tcp"]);
});

test("missing Xvfb or command fails closed instead of using an existing display", (t) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "aego-no-xvfb-test-"));
    t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
    const noCommand = spawnSync("/bin/sh", [wrapper], {encoding:"utf8", env:{PATH:dir, DISPLAY:":user-desktop"}});
    assert.equal(noCommand.status, 2);
    const missing = spawnSync("/bin/sh", [wrapper, process.execPath, "-e", "process.exit(0)"], {
        encoding:"utf8", env:{PATH:dir, DISPLAY:":user-desktop"},
    });
    assert.equal(missing.status, 1);
    assert.match(missing.stderr, /require xvfb-run/);
});
