// Main-thread half of the audio Worker transport. It owns one Worker and one
// outstanding RPC. The caller owns the mixer and startup/RPC deadlines, applies
// batches before acking them and handles failures.
'use strict';

(() => {
    const MAX_TRANSFER = 1 << 16, MAX_JSON = 1 << 20;

    class AegoAudioWorkerHost {
        constructor(worker, notify, Channel = MessageChannel) {
            if (!worker || typeof worker.postMessage !== 'function' || typeof worker.terminate !== 'function' ||
                typeof notify !== 'function' || typeof Channel !== 'function') throw new Error('invalid audio worker host');
            this.worker = worker;
            this.notify = notify;
            this.Channel = Channel;
            this.state = 'new';
            this.attached = false;
            this.serial = 0;
            this.pending = null;
            this.batch = '';
            this.port = null;
            worker.onmessage = event => this.message(event.data);
            worker.onerror = event => {
                if (event && event.preventDefault) event.preventDefault();
                this.fail(event && event.message || 'audio worker error');
            };
            worker.onmessageerror = () => this.fail('audio worker message could not be decoded');
        }

        emit(message) {
            try { this.notify(message); }
            catch (error) { this.fail(error && error.message || 'audio worker callback failed'); }
        }

        fail(error) {
            if (this.state === 'failed' || this.state === 'closed') return;
            const attached = this.attached;
            this.state = 'failed';
            this.terminate();
            try { this.notify({type:'fatal', error:String(error), attached}); }
            catch (_) { /* The transport is already stopped. */ }
        }

        terminate() {
            if (this.port) {
                try { this.port.close(); } catch (_) { /* Already transferred or closed. */ }
                this.port = null;
            }
            if (this.worker) {
                this.worker.onmessage = this.worker.onerror = this.worker.onmessageerror = null;
                try { this.worker.terminate(); } catch (_) { /* Already stopped. */ }
                this.worker = null;
            }
        }

        // Until the worker reports ready, the second channel endpoint stays on
        // this thread and the AudioWorklet remains usable by the local backend.
        // Once attachment is attempted, failure is conservatively fatal: the
        // caller must recreate the output rather than guess whether it arrived.
        start({module, node, input, block, target, data, index}) {
            if (this.state !== 'new' || !node || !node.port || typeof node.port.postMessage !== 'function' ||
                typeof input !== 'string') throw new Error('invalid audio worker startup');
            let channel;
            try { channel = new this.Channel(); }
            catch (error) { this.fail(error && error.message || error); return; }
            this.port = channel.port2;
            this.node = node;
            this.state = 'starting';
            try {
                this.worker.postMessage({type:'init', module, port:channel.port1, input, block, target, data, index}, [channel.port1]);
            } catch (error) {
                try { channel.port1.close(); } catch (_) { /* No ownership remains. */ }
                this.fail(error && error.message || error);
            }
        }

        attach(node, quiet = false) {
            if (!this.port || !node || !node.port || typeof node.port.postMessage !== 'function') {
                throw new Error('invalid audio output attachment');
            }
            const port = this.port;
            this.attached = true;
            node.port.postMessage({producerPort:port, quiet}, [port]);
            this.port = null;
            this.node = node;
        }

        message(message) {
            if (this.state === 'failed' || this.state === 'closed') return;
            try {
                if (!message || typeof message !== 'object') throw new Error('invalid audio worker message');
                if (message.type === 'fatal') throw new Error(String(message.error || 'audio worker failed'));
                if (message.type === 'ready') {
                    if (this.state !== 'starting') throw new Error('unexpected audio worker readiness');
                    this.attach(this.node);
                    this.state = 'ready';
                    this.emit({type:'ready'});
                    return;
                }
                if (this.state !== 'ready') throw new Error('audio worker message before startup');
                if (message.type === 'batch') {
                    if (this.batch || typeof message.batch !== 'string' || message.batch.length > MAX_JSON) {
                        throw new Error('invalid or overlapping audio batch');
                    }
                    const value = JSON.parse(message.batch);
                    if (!value || typeof value.serial !== 'string' || !/^[1-9][0-9]*$/.test(value.serial)) {
                        throw new Error('invalid audio batch serial');
                    }
                    this.batch = value.serial;
                    this.emit({type:'batch', batch:message.batch, stats:message.stats});
                    return;
                }
                if (message.type !== 'reply' || !this.pending || message.id !== this.pending.id ||
                    typeof message.result !== 'string' || message.error !== undefined) {
                    throw new Error(message.error || 'invalid audio worker reply');
                }
                const pending = this.pending;
                if (pending.node) this.attach(pending.node, pending.quiet);
                this.pending = null;
                this.emit({type:'reply', id:message.id, result:message.result});
            } catch (error) { this.fail(error && error.message || error); }
        }

        nextID() {
            if (this.state !== 'ready' || this.pending || this.serial >= Number.MAX_SAFE_INTEGER) {
                throw new Error('audio worker already has a request in flight');
            }
            return ++this.serial;
        }

        // PCM is copied before transfer; neither a detached ArrayBuffer nor a
        // later producer write can change the acknowledged source data.
        send(op, input, pcm, offset) {
            if (typeof input !== 'string' || input.length > MAX_JSON ||
                (!(Number.isInteger(op) && op >= 2 && op <= 14 && op !== 10) && op !== 'memory-chunk') ||
                (op === 'memory-chunk' && (!Number.isSafeInteger(offset) || offset < 0))) {
                throw new Error('invalid audio worker request');
            }
            let copy;
            if (pcm !== undefined && pcm !== null) {
                if (!(pcm instanceof Float32Array) || !pcm.length || pcm.length > MAX_TRANSFER) {
                    throw new Error('invalid audio worker PCM');
                }
                copy = new Float32Array(pcm);
            }
            const id = this.nextID();
            this.pending = {id};
            try {
                this.worker.postMessage({type:'rpc', id, op, input, pcm:copy, offset}, copy ? [copy.buffer] : []);
            } catch (error) { this.fail(error && error.message || error); }
            return id;
        }

        // Completion means the worklet has discarded the old queue and the
        // Worker has stopped refilling it. Existing worklet statistics remain.
        quiesce() {
            const id = this.nextID();
            this.pending = {id};
            try {
                this.worker.postMessage({type:'rpc', id, op:'quiesce'});
            } catch (error) { this.fail(error && error.message || error); }
            return id;
        }

        // The Worker primes fresh PCM before the worklet resumes consumption.
        resume() {
            const id = this.nextID();
            this.pending = {id};
            try {
                this.worker.postMessage({type:'rpc', id, op:'resume'});
            } catch (error) { this.fail(error && error.message || error); }
            return id;
        }

        // Disconnect the old worklet first. The caller must serialize this
        // operation with commands and retain the same live mixer and Worker.
        rebind({node, block, target, data, index, rate, quiet = false}) {
            if (!node || node === this.node || !node.port || typeof node.port.postMessage !== 'function') {
                throw new Error('invalid audio output rebind');
            }
            if (typeof quiet !== 'boolean') throw new Error('invalid audio output quiet state');
            const id = this.nextID();
            let channel;
            try { channel = new this.Channel(); }
            catch (error) { this.fail(error && error.message || error); return id; }
            this.port = channel.port2;
            this.pending = {id, node, quiet};
            try {
                this.worker.postMessage({type:'rpc', id, op:'rebind', port:channel.port1, block, target, data, index, rate, quiet}, [channel.port1]);
            } catch (error) {
                try { channel.port1.close(); } catch (_) { /* No ownership remains. */ }
                this.fail(error && error.message || error);
            }
            return id;
        }

        ackBatch(serial) {
            if (this.state !== 'ready') throw new Error('audio worker is not ready');
            if (!this.batch || serial !== this.batch) {
                this.fail('invalid audio batch acknowledgement');
                throw new Error('invalid audio batch acknowledgement');
            }
            try {
                this.worker.postMessage({type:'batch-ack', serial});
                this.batch = '';
            } catch (error) { this.fail(error && error.message || error); }
        }

        close() {
            if (this.state === 'closed') return;
            this.state = 'closed';
            this.terminate();
            this.pending = null;
            this.batch = '';
        }
    }

    // The page loads this before Go starts. Failure leaves the existing local
    // backend available; no mixer or output ownership has been transferred.
    async function prepareAudioWorker(reactorURL, workerURL, timeoutMS = 8000) {
        if (typeof Worker !== 'function' || typeof WebAssembly !== 'object' || typeof WebAssembly.compile !== 'function' ||
            typeof AbortController !== 'function' || !Number.isSafeInteger(timeoutMS) || timeoutMS <= 0 || timeoutMS > 60000) return null;
        let timer, controller;
        try {
            const reactor = new URL(reactorURL, location.href), worker = new URL(workerURL, location.href);
            if (!['http:', 'https:'].includes(reactor.protocol) || !['http:', 'https:'].includes(worker.protocol) ||
                reactor.origin !== location.origin || worker.origin !== location.origin ||
                reactor.username || reactor.password || worker.username || worker.password) {
                throw new Error('audio worker must be same-origin');
            }
            controller = new AbortController();
            const timeout = new Promise((_, reject) => {
                timer = setTimeout(() => { controller.abort(); reject(new Error('audio reactor timed out')); }, timeoutMS);
            });
            const load = async () => {
                // No cross-origin redirects, credentials in URLs, or opaque
                // responses may expand the page's Worker trust boundary.
                const response = await fetch(reactor.href, {signal:controller.signal, mode:'same-origin', redirect:'error'});
                if (!response.ok) throw new Error(`audio reactor HTTP ${response.status}`);
                if (response.redirected || (response.url && new URL(response.url).origin !== location.origin)) {
                    throw new Error('audio reactor must not redirect');
                }
                const limit = 32 << 20;
                const declared = Number(response.headers && response.headers.get('Content-Length'));
                if (Number.isFinite(declared) && declared > limit) throw new Error('audio reactor exceeds its size limit');
                let bytes;
                if (response.body && typeof response.body.getReader === 'function') {
                    const reader = response.body.getReader(), parts = [];
                    let size = 0;
                    for (;;) {
                        const {done, value} = await reader.read();
                        if (done) break;
                        if (!(value instanceof Uint8Array) || value.byteLength > limit - size) throw new Error('audio reactor exceeds its size limit');
                        size += value.byteLength;
                        parts.push(value);
                    }
                    if (!size) throw new Error('audio reactor is empty');
                    bytes = new Uint8Array(size);
                    let at = 0;
                    for (const part of parts) { bytes.set(part, at); at += part.byteLength; }
                } else {
                    if (!Number.isSafeInteger(declared) || declared <= 0 || declared > limit) throw new Error('audio reactor size is unavailable');
                    bytes = await response.arrayBuffer();
                    if (!bytes.byteLength || bytes.byteLength > limit) throw new Error('audio reactor exceeds its size limit');
                }
                const module = await WebAssembly.compile(bytes);
                return {module, url:worker.href};
            };
            return await Promise.race([load(), timeout]);
        } catch (error) {
            if (controller) controller.abort();
            if (globalThis.console && console.warn) console.warn('Aego audio worker unavailable; using local audio:', error);
            return null;
        } finally {
            if (timer !== undefined) clearTimeout(timer);
        }
    }

    globalThis.AegoAudioWorkerHost = AegoAudioWorkerHost;
    globalThis.AegoPrepareAudioWorker = prepareAudioWorker;
})();
