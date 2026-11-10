class AegoOut extends AudioWorkletProcessor {
    constructor(options) {
        super();
        const o = options.processorOptions;
        this.ch = o.channels;
        this.report = o.report;
        this.shared = !!o.data;
        if (this.shared) {
            this.data = new Float32Array(o.data);
            this.index = new Int32Array(o.index);
            this.mask = this.data.length / this.ch - 1;
        }
        this.blocks = [];
        this.block = null;
        this.at = 0;
        this.primed = false;
        this.queued = 0;
        this.refillAt = o.refillAt || 0;
        this.refillArmed = false;
        this.refillSequence = 0;
        this.refillPending = 0;
        this.consumed = 0;
        this.underruns = 0;
        this.sum = 0;
        this.count = 0;
        this.since = 0;
        this.producer = null;
        const enqueue = data => {
            if (!(data instanceof Float32Array) || data.length % this.ch !== 0) throw new Error('invalid audio PCM block');
            this.queued += data.length / this.ch;
            this.blocks.push(data);
        };
        this.port.onmessage = ({data}) => {
            if (data && data.producerPort) {
                if (this.producer) throw new Error('audio producer already attached');
                this.producer = data.producerPort;
                this.producer.onmessage = ({data}) => {
                    if (data && Number.isSafeInteger(data.refilled) && data.refilled > 0) {
                        if (data.refilled === this.refillPending) {
                            this.refillPending = 0;
                            this.refillArmed = true;
                        }
                    } else {
                        enqueue(data);
                    }
                };
                if (this.producer.start) this.producer.start();
                this.producer.postMessage({ready:true});
            } else if (data && typeof data.refillAt === "number") {
                this.refillAt = data.refillAt;
            } else {
                if (this.producer) throw new Error('audio PCM has multiple producers');
                enqueue(data);
            }
        };
    }

    next() {
        if (this.block && this.at < this.block.length) return true;
        this.block = this.blocks.length ? this.blocks.shift() : null;
        this.at = 0;
        return this.block !== null;
    }

    process(inputs, outputs) {
        const out = outputs[0];
        const left = out[0];
        const right = out.length > 1 ? out[1] : null;
        const n = left.length;
        const ch = this.ch;
        let got = 0;
        let remaining = 0;
        if (this.shared) {
            const head = Atomics.load(this.index, 0);
            const avail = (Atomics.load(this.index, 1) - head) | 0;
            got = Math.min(n, avail);
            for (let f = 0; f < got; f++) {
                const k = ((head + f) & this.mask) * ch;
                const l = this.data[k];
                const r = ch > 1 ? this.data[k + 1] : l;
                left[f] = l;
                if (right) right[f] = r;
                this.sum += l * l + r * r;
            }
            Atomics.store(this.index, 0, (head + got) | 0);
            remaining = avail - got;
        } else {
            while (got < n && this.next()) {
                const b = this.block;
                while (got < n && this.at < b.length) {
                    const l = b[this.at];
                    const r = ch > 1 ? b[this.at + 1] : l;
                    left[got] = l;
                    if (right) right[got] = r;
                    this.sum += l * l + r * r;
                    this.at += ch;
                    got++;
                }
            }
        }
        if (!this.shared) remaining = this.queued -= got;
        for (let f = got; f < n; f++) {
            left[f] = 0;
            if (right) right[f] = 0;
        }
        if (got > 0) this.primed = true;
        if (got < n && this.primed) this.underruns++;
        this.consumed += got;
        this.count += got * 2;
        this.since += n;
        // Keep the regular reports. If the producer misses its usual frame,
        // request the next fill before silence, only once per threshold crossing.
        if (remaining > this.refillAt && !this.refillPending) this.refillArmed = true;
        const need = this.refillAt > 0 && this.refillArmed && !this.refillPending && remaining <= this.refillAt;
        if (need) this.refillArmed = false;
        if (this.since >= this.report || need) {
            this.since = 0;
            const report = { consumed: this.consumed, time: currentTime + n / sampleRate, underruns: this.underruns, sum: this.sum, count: this.count };
            if (need && this.producer) {
                this.refillPending = ++this.refillSequence;
                report.refill = this.refillPending;
            }
            this.port.postMessage(report);
            if (this.producer) this.producer.postMessage(report);
        }
        return true;
    }
}

registerProcessor("aego-out", AegoOut);
