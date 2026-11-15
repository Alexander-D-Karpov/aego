import assert from "node:assert/strict";
import test from "node:test";
import {waitForPersistCycle} from "./persist-cycle.mjs";

for (const order of [["game saved", "game loaded"], ["game loaded", "game saved"]]) {
    test(`waits for both events: ${order.join(" then ")}`, async () => {
        let clock = 0, steps = 0;
        const batches = [[], [{text: order[0]}], [{text: order[1]}], []];
        await waitForPersistCycle({
            readLog: async () => batches.shift() || [],
            advance: async remaining => { assert.ok(remaining > 0); steps++; clock += 1; },
            timeout: 10, now: () => clock,
        });
        assert.equal(steps, 3); // Includes a frame advance after both completions.
    });
}

test("a missing completion cannot pass, even if many frames advance", async () => {
    let clock = 0;
    await assert.rejects(waitForPersistCycle({
        readLog: async () => [{text: "game saved"}],
        advance: async () => { clock++; }, timeout: 3, now: () => clock,
    }), /saved=true loaded=false/);
});

test("runtime errors are not lost before or after completion", async () => {
    for (const batches of [
        [[{text: "failure", error: true}]],
        [[{text: "game saved"}, {text: "game loaded"}], [{text: "late failure", error: true}]],
    ]) {
        await assert.rejects(waitForPersistCycle({readLog: async () => batches.shift() || [], advance: async () => {}}), /runtime error/);
    }
});

test("malformed logs and advance failures fail closed", async () => {
    await assert.rejects(waitForPersistCycle({readLog: async () => null, advance: async () => {}}), /invalid log/);
    await assert.rejects(waitForPersistCycle({readLog: async () => [], advance: async () => { throw new Error("advance failed"); }}), /advance failed/);
});
