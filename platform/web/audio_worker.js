// The WASI mixer runs in a dedicated Worker, never in the AudioWorklet. This
// is the worker-side transport; the browser backend must not select it until
// startup, source lifetime, mute/visibility and failure handling are wired.
'use strict';

(() => {
    const OP_INIT = 1, OP_BEGIN_MEMORY = 2, OP_COMMIT_MEMORY = 3;
    const OP_STREAM_WRITE = 5, OP_DROP = 9, OP_DRAIN = 10, OP_SKIP = 11, OP_SET_SAMPLE_RATE = 13;
    const ABI_VERSION = 2, MAX_JSON = 1 << 20, MAX_TRANSFER = 1 << 16;
    const encoder = new TextEncoder(), decoder = new TextDecoder();
    let exports, port, shared, ring, index, block = 0, target = 0, rate = 0;
    let tail = 0, sent = 0, consumed = 0, started = false, failed = false;
    let staging = null, lastID = 0, blocks = 0, maxMixMS = 0;
    let batchPending = '', batchDirty = false;
    class ReactorFailure extends Error {}

    function reactor(fn) {
        try { return fn(); }
        catch (error) { throw new ReactorFailure(String(error && error.message || error)); }
    }

    function wasiFor(instance) {
        const memory = () => instance.exports.memory.buffer;
        const put32 = (ptr, value) => new DataView(memory()).setUint32(ptr, value, true);
        return {
            args_sizes_get: (count, size) => { put32(count, 0); put32(size, 0); return 0; },
            args_get: () => 0,
            environ_sizes_get: (count, size) => { put32(count, 0); put32(size, 0); return 0; },
            environ_get: () => 0,
            random_get: (ptr, size) => {
                const bytes = new Uint8Array(memory(), ptr, size);
                for (let at = 0; at < size; at += 65536) crypto.getRandomValues(bytes.subarray(at, at + 65536));
                return 0;
            },
            clock_time_get: (clock, _precision, ptr) => {
                const ms = performance.now() + (clock === 0 ? performance.timeOrigin : 0);
                new DataView(memory()).setBigUint64(ptr, BigInt(Math.floor(ms * 1e6)), true);
                return 0;
            },
            fd_write: (_fd, iov, count, written) => {
                const view = new DataView(memory());
                let size = 0;
                for (let i = 0; i < count; i++) size += view.getUint32(iov + i * 8 + 4, true);
                put32(written, size);
                return 0;
            },
            fd_read: () => 8, fd_close: () => 8, fd_fdstat_get: () => 8,
            fd_fdstat_set_flags: () => 8, fd_prestat_get: () => 8, fd_prestat_dir_name: () => 8,
            sched_yield: () => 0,
            poll_oneoff: () => { throw new Error('blocking WASI poll is unavailable in the audio worker'); },
            proc_exit: code => { throw new Error(`audio reactor exited ${code}`); },
        };
    }

    function bytes(ptr, length) { return new Uint8Array(exports.memory.buffer, ptr, length); }
    function reactorError() { return decoder.decode(bytes(exports.aego_error(), exports.aego_error_size())); }
    function call(op, input) {
        if (typeof input !== 'string') throw new Error('audio request must be exact JSON text');
        const data = encoder.encode(input);
        if (!data.length || data.length > MAX_JSON) throw new Error('audio request exceeds its limit');
        return reactor(() => {
            const ptr = exports.aego_input(data.length);
            if (!ptr) throw new Error(reactorError() || 'audio input allocation failed');
            bytes(ptr, data.length).set(data);
            if (!exports.aego_call(op)) throw new Error(reactorError() || 'audio operation failed');
            const size = exports.aego_result_size();
            return size ? decoder.decode(bytes(exports.aego_result(), size)) : '';
        });
    }
    function request(input) {
        const value = JSON.parse(input);
        if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('invalid audio request');
        return value;
    }
    function sourceID(value) {
        return typeof value === 'string' && /^[1-9][0-9]*$/.test(value);
    }
    function copyMemory(message) {
        if (!staging || !(message.pcm instanceof Float32Array) || !message.pcm.length ||
            message.pcm.length > MAX_TRANSFER || message.offset !== staging.received ||
            message.offset + message.pcm.length > staging.samples) throw new Error('invalid staged PCM chunk');
        reactor(() => {
            const ptr = exports.aego_memory();
            if (!ptr) throw new Error(reactorError() || 'staged PCM is unavailable');
            new Float32Array(exports.memory.buffer, ptr + message.offset * 4, message.pcm.length).set(message.pcm);
        });
        staging.received += message.pcm.length;
    }
    function invoke(message) {
        if (message.op === 'rebind') {
            rebind(message);
            return '';
        }
        if (message.op === 'memory-chunk') {
            copyMemory(message);
            return '';
        }
        if (!Number.isInteger(message.op) || message.op < OP_BEGIN_MEMORY || message.op > 14 || message.op === OP_DRAIN) {
            throw new Error('unsupported audio operation');
        }
        const r = request(message.input);
        if (message.op === OP_BEGIN_MEMORY) {
            if (staging || !sourceID(r.source) || !Number.isSafeInteger(r.samples) || r.samples < 0) {
                throw new Error('invalid staged source');
            }
            const result = call(message.op, message.input);
            staging = {source:r.source, samples:r.samples, received:0};
            return result;
        }
        if (message.op === OP_COMMIT_MEMORY) {
            if (!staging || r.source !== staging.source || staging.received !== staging.samples) {
                throw new Error('staged PCM is incomplete');
            }
            const result = call(message.op, message.input);
            staging = null;
            return result;
        }
        if (message.op === OP_STREAM_WRITE) {
            const pcm = message.pcm;
            if (!(pcm instanceof Float32Array) || !pcm.length || pcm.length > MAX_TRANSFER || r.samples !== pcm.length) {
                throw new Error('invalid streaming PCM chunk');
            }
            reactor(() => {
                const ptr = exports.aego_stream(pcm.length);
                if (!ptr) throw new Error(reactorError() || 'stream buffer is unavailable');
                new Float32Array(exports.memory.buffer, ptr, pcm.length).set(pcm);
            });
        }
        const result = call(message.op, message.input);
        if (message.op === OP_DROP && staging && r.source === staging.source) staging = null;
        if (message.op === OP_SET_SAMPLE_RATE) rate = r.rate;
        return result;
    }
    function fail(error) {
        if (failed) return;
        failed = true;
        postMessage({type:'fatal', error:String(error && error.message || error)});
    }
    function queued() {
        return shared ? ((tail - Atomics.load(index, 0)) | 0) : sent - consumed;
    }
    function publishBatch() {
        if (batchPending || !batchDirty) return;
        const batch = call(OP_DRAIN, '{}');
        if (!batch) throw new Error('audio batch missing');
        const serial = JSON.parse(batch).serial;
        if (typeof serial !== 'string' || !/^[1-9][0-9]*$/.test(serial)) throw new Error('invalid audio batch serial');
        batchPending = serial;
        batchDirty = false;
        postMessage({type:'batch', batch});
    }
    function fill() {
        if (!started || failed) return;
        const q = queued();
        const capacity = shared ? ring.length / 2 : target + block;
        if (!Number.isSafeInteger(q) || q < 0 || q > capacity) throw new Error('invalid audio queue state');
        let count = Math.max(0, Math.ceil((target - q) / block));
        if (shared) count = Math.min(count, Math.floor((capacity - q) / block));
        if (!count) return;
        const pcm = new Float32Array(count * block * 2);
        for (let i = 0; i < count; i++) {
            const before = performance.now(), ptr = exports.aego_render(block);
            if (!ptr) throw new Error(reactorError() || 'audio render failed');
            maxMixMS = Math.max(maxMixMS, performance.now() - before);
            pcm.set(new Float32Array(exports.memory.buffer, ptr, block * 2), i * block * 2);
        }
        if (shared) {
            const frames = pcm.length / 2, mask = ring.length / 2 - 1;
            if (frames > capacity - q) throw new Error('shared audio ring is full');
            for (let i = 0; i < frames; i++) {
                const at = ((tail + i) & mask) * 2;
                ring[at] = pcm[i * 2];
                ring[at + 1] = pcm[i * 2 + 1];
            }
            tail = (tail + frames) | 0;
            Atomics.store(index, 1, tail);
        } else {
            port.postMessage(pcm, [pcm.buffer]);
        }
        sent += count * block;
        blocks += count;
        // Keep mixing while the page is blocked, but never queue an
        // unbounded series of state batches for it. Until the one outstanding
        // batch is acknowledged, the mixer retains its bounded event and
        // release ledgers and publishes their combined state on the next ack.
        batchDirty = true;
        publishBatch();
    }
    function onOutput(boundPort, event) {
        // Reports already queued for an old device must never advance the
        // counters or refill the new one after a rebind.
        if (boundPort !== port) return;
        try {
            const data = event.data;
            if (data && data.ready === true) {
                started = true;
            } else {
                if (!data || !Number.isSafeInteger(data.consumed) || data.consumed < consumed || data.consumed > sent) {
                    throw new Error('invalid audio consumption report');
                }
                if (data.refill !== undefined && (!Number.isSafeInteger(data.refill) || data.refill <= 0)) {
                    throw new Error('invalid audio refill request');
                }
                consumed = data.consumed;
            }
            fill();
            // Acknowledge after publishing PCM. The worklet can request again
            // even if a delayed refill did not rise above its threshold, with
            // at most one threshold request outstanding while we are stalled.
            if (data && data.refill !== undefined) boundPort.postMessage({refilled:data.refill});
        } catch (error) { fail(error); }
    }
    function outputConfig(data) {
        if (!data.port || typeof data.port.postMessage !== 'function' || typeof data.port.close !== 'function' ||
            !Number.isInteger(data.block) || data.block < 1 || data.block > 2048 ||
            !Number.isInteger(data.target) || data.target < data.block || data.target > 24576 || data.target % data.block !== 0) {
            throw new Error('invalid audio worker output');
        }
        let nextRing, nextIndex;
        const nextShared = !!data.data;
        if (nextShared) {
            if (typeof SharedArrayBuffer === 'undefined' || !(data.data instanceof SharedArrayBuffer) ||
                !(data.index instanceof SharedArrayBuffer)) throw new Error('invalid shared audio buffers');
            nextRing = new Float32Array(data.data);
            nextIndex = new Int32Array(data.index);
            const frames = nextRing.length / 2;
            if (nextIndex.length < 2 || !Number.isInteger(frames) || frames < data.target || (frames & (frames - 1)) !== 0 ||
                Atomics.load(nextIndex, 0) !== 0 || Atomics.load(nextIndex, 1) !== 0) {
                throw new Error('invalid shared audio ring');
            }
        } else if (data.index) {
            throw new Error('audio index has no shared ring');
        }
        return {port:data.port, shared:nextShared, ring:nextRing, index:nextIndex, block:data.block, target:data.target};
    }
    function bindOutput(out) {
        port = out.port;
        shared = out.shared;
        ring = out.ring;
        index = out.index;
        block = out.block;
        target = out.target;
        tail = sent = consumed = 0;
        started = false;
        const boundPort = port;
        port.onmessage = event => onOutput(boundPort, event);
        if (port.start) port.start();
    }
    function rebind(data) {
        const next = outputConfig(data);
        if (next.port === port || !Number.isInteger(data.rate) || data.rate <= 0 || data.rate > 768000) {
            throw new Error('invalid audio worker rebind');
        }
        // The caller must disconnect the old AudioWorklet first. Validate the
        // new output before touching the current mixer or its existing port.
        reactor(() => {
            if (data.rate !== rate) call(OP_SET_SAMPLE_RATE, JSON.stringify({rate:data.rate}));
            rate = data.rate;
            port.onmessage = null;
            port.close();
            bindOutput(next);
        });
    }
    function init(data) {
        if (exports || failed) throw new Error('invalid audio worker initialization');
        const next = outputConfig(data);
        const options = request(data.input);
        let instance;
        instance = new WebAssembly.Instance(data.module, {wasi_snapshot_preview1:wasiFor({get exports() { return instance.exports; }})});
        exports = instance.exports;
        exports._initialize();
        if (typeof exports.aego_abi_version !== 'function' || exports.aego_abi_version() !== ABI_VERSION) {
            throw new Error('audio worker ABI version mismatch');
        }
        call(OP_INIT, data.input);
        rate = options.rate;
        bindOutput(next);
        postMessage({type:'ready'});
    }
    onmessage = event => {
        const message = event.data;
        try {
            if (!message || typeof message !== 'object') throw new Error('invalid audio worker message');
            if (message.type === 'init') { init(message); return; }
            if (message.type === 'close') {
                failed = true;
                if (port) { port.onmessage = null; port.close(); }
                close();
                return;
            }
            if (message.type === 'batch-ack') {
                if (!exports || failed || !batchPending || message.serial !== batchPending) {
                    throw new Error('invalid audio batch acknowledgement');
                }
                batchPending = '';
                publishBatch();
                return;
            }
            if (!exports || failed || message.type !== 'rpc' || !Number.isSafeInteger(message.id) || message.id !== lastID + 1) {
                throw new Error('invalid audio worker request order');
            }
            lastID = message.id;
            let result;
            try {
                result = invoke(message);
            } catch (error) {
                postMessage({type:'reply', id:message.id, error:String(error && error.message || error)});
                // An ABI failure can occur after the mixer has partially
                // changed state. Only local validation errors are retryable.
                if (error instanceof ReactorFailure) fail(error);
                return;
            }
            postMessage({type:'reply', id:message.id, result});
            if (message.op === OP_SKIP) {
                batchDirty = true;
                publishBatch();
            }
            fill();
        } catch (error) { fail(error); }
    };
})();
