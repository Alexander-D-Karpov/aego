// Wait for the actual asynchronous save and load; log reads consume their entries.
export async function waitForPersistCycle({readLog, advance, timeout = 10000, now = () => performance.now()}) {
    const deadline = now() + timeout;
    let saved = false, loaded = false;
    async function collect() {
        const lines = await readLog();
        if (!Array.isArray(lines)) throw new Error("save.Persist returned an invalid log");
        for (const line of lines) {
            if (line.error) throw new Error(`save.Persist runtime error: ${line.text}`);
            saved ||= line.text === "game saved";
            loaded ||= line.text === "game loaded";
        }
    }
    function remaining() {
        const ms = deadline - now();
        if (ms <= 0) throw new Error(`save.Persist did not complete: saved=${saved} loaded=${loaded}`);
        return Math.max(1, Math.ceil(ms));
    }
    for (;;) {
        await collect();
        if (saved && loaded) {
            await advance(remaining());
            await collect();
            remaining();
            return;
        }
        await advance(remaining());
    }
}
