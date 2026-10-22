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
        this.consumed = 0;
        this.underruns = 0;
        this.sum = 0;
        this.count = 0;
        this.since = 0;
        this.port.onmessage = (e) => this.blocks.push(e.data);
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
        for (let f = got; f < n; f++) {
            left[f] = 0;
            if (right) right[f] = 0;
        }
        if (got > 0) this.primed = true;
        if (got < n && this.primed) this.underruns++;
        this.consumed += got;
        this.count += got * 2;
        this.since += n;
        if (this.since >= this.report) {
            this.since = 0;
            this.port.postMessage({ consumed: this.consumed, time: currentTime + n / sampleRate, underruns: this.underruns, sum: this.sum, count: this.count });
        }
        return true;
    }
}

registerProcessor("aego-out", AegoOut);
