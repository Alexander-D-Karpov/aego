import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { createHash } from "node:crypto";
import { PNG } from "pngjs";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright-core");

const [rootArg, routeArg, outArg] = process.argv.slice(2);
if (!outArg) throw Error("usage: node agent-game.mjs <web-export> <route.aeinput> <captures>");
const root = path.resolve(rootArg), out = path.resolve(outArg);
const manifest = fs.readFileSync(path.join(root, "export.aetext"), "utf8");
if (!/module="bellrun"/.test(manifest) || !/target="web"/.test(manifest)) throw Error("expected the exported bellrun project");
const route = fs.readFileSync(routeArg), wasm = fs.readFileSync(path.join(root, "game.wasm"));
const sha = x => createHash("sha256").update(x).digest("hex");
const mime = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = http.createServer((req,res) => {
    const rel = new URL(req.url,"http://localhost").pathname.replace(/^\/+/, "") || "index.html";
    const f = path.resolve(root,rel);
    if (!f.startsWith(root + path.sep) || !fs.statSync(f,{throwIfNoEntry:false})?.isFile()) { res.writeHead(404).end(); return; }
    res.setHeader("Content-Type",mime[path.extname(f)] ?? "application/octet-stream");
    fs.createReadStream(f).pipe(res);
});
await new Promise(r => server.listen(0,"127.0.0.1",r));
fs.mkdirSync(out,{recursive:true});
let browser;
try {
    browser = await chromium.launch({executablePath: process.env.CHROMIUM, args: ["--use-angle=swiftshader","--enable-unsafe-swiftshader"]});
    const page = await browser.newPage({ viewport:{width:1280,height:720}, deviceScaleFactor:1 });
    const errors = [];
    page.on("pageerror",e=>errors.push(e.message));
    await page.addInitScript(data=>{
        globalThis.aegoReplay=data;
        const r=globalThis.requestAnimationFrame.bind(globalThis);
        const s=globalThis.routeCapture={target:12,held:false,pending:[],started:false};
        function frame(cb){return r(ts=>{
            if (s.held){s.pending.push(cb);return;}
            if (globalThis.aegoGame && !s.started){s.started=true;aegoGame.lockstep(true);}
            cb(ts);
            if (s.started && aegoGame.frames()>=s.target) s.held=true;
        });}
        globalThis.requestAnimationFrame=cb=>s.held?(s.pending.push(cb),0):frame(cb);
        s.next=t=>{ s.target=t;s.held=false;for(const cb of s.pending.splice(0))frame(cb);};
    },route.toString("base64"));
    await page.goto("http://127.0.0.1:"+server.address().port+"/?seed=41&bench=1");
    const stats=[];
    for(const tick of [12,74,175,184]){
        if(tick!==12) await page.evaluate(t=>routeCapture.next(t),tick);
        await page.waitForFunction(t=>globalThis.routeCapture.held && aegoGame.frames()===t,tick,{polling:40,timeout:90000});
        await page.evaluate(()=>{for(const id of ["mute","fullscreen"]){const e=document.getElementById(id);if(e)e.hidden=true;}});
        const bytes=await page.locator("#game").screenshot();
        const img=PNG.sync.read(bytes);
        if(img.width!==1280||img.height!==720)throw Error("unexpected game viewport");
        const near=(wx,wy)=>{
            const x=Math.round(640+wx*64), y=Math.round(360+wy*64);let gold=0;
            for(let dy=-10;dy<=10;dy++)for(let dx=-10;dx<=10;dx++){const i=((y+dy)*img.width+x+dx)*4;const [r,g,b]=img.data.subarray(i,i+3);if(r>=240&&g>=174&&g<=202&&b>=70&&b<=105)gold++;}
            return gold>10;
        };
        const coins=[near(-3.5,1.35),near(-.8,.25),near(2.5,1.35)],win=near(5.2,-1.3);
        const expected=tick===175?[false,false,false]:tick===74?[false,false,true]:[true,true,true];
        if(coins.some((c,i)=>c!==expected[i])||win!==(tick===175))throw Error("gameplay pixels mismatch at "+tick+": "+JSON.stringify({coins,win}));
        fs.writeFileSync(path.join(out,"tick-"+tick+".png"),bytes);
        stats.push({tick,coins,win,pixelSHA256:sha(img.data)});
    }
    if(errors.length)throw Error(errors.join("; "));
    fs.writeFileSync(path.join(out,"web-replay.json"),JSON.stringify({browser:browser.version(),wasm:sha(wasm),route:sha(route),stats},null,2)+"\n");
} finally { if(browser)await browser.close();await new Promise(r=>server.close(r)); }
