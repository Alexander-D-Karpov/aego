// Actual GL texture-array/HDR/context-restore checks; missing browser/context is
// a failure, while unavailable optional driver extensions are explicit.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
const { chromium, firefox } = await import(process.env.PLAYWRIGHT_MODULE ?? "playwright-core");
const options = Object.fromEntries(process.argv.slice(2).map(arg => {
    const at = arg.indexOf("=");
    return at < 0 ? [arg.replace(/^--/, ""), "true"] : [arg.slice(2, at), arg.slice(at + 1)];
}));
const root = path.resolve(options.site ?? "../../.cache/web-gpu");
const mime = {".html":"text/html", ".js":"text/javascript", ".wasm":"application/wasm"};
const server = http.createServer((req,res) => {
    const relative = decodeURIComponent(new URL(req.url,"http://localhost").pathname).replace(/^\/+/, "") || "index.html";
    const file = path.resolve(root,relative);
    if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {res.writeHead(404);res.end();return;}
    res.writeHead(200,{"Content-Type":mime[path.extname(file)]??"application/octet-stream"});
    fs.createReadStream(file).pipe(res);
});
await new Promise(resolve => server.listen(0,"127.0.0.1",resolve));
const reports=[];
try {
    for (const name of (options.browsers??"chromium,firefox").split(",")) {
        let browser;
        try {
            if (!["chromium","firefox"].includes(name)) throw new Error(`unsupported browser ${name}`);
            const isFirefox = name === "firefox";
            browser = await (isFirefox ? firefox : chromium).launch({
                executablePath: isFirefox ? process.env.FIREFOX : process.env.CHROMIUM ?? "/usr/bin/chromium",
                headless: !isFirefox || !process.env.DISPLAY,
                args: isFirefox ? [] : ["--no-sandbox", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
                firefoxUserPrefs: isFirefox ? {"webgl.force-enabled":true,"webgl.disabled":false} : undefined,
            });
            const page = await browser.newPage({viewport:{width:800,height:600},deviceScaleFactor:1});
            const errors=[];
            page.on("pageerror",error=>errors.push(error.message));
            if(process.env.TRACE_WEBGL)page.on("console",message=>console.error(message.type(),message.text()));
            await page.goto(`http://127.0.0.1:${server.address().port}/`);
            await page.waitForFunction(()=>typeof globalThis.aegoGPUResults==="string",null,{timeout:120000});
            const result = await page.evaluate(()=>JSON.parse(globalThis.aegoGPUResults));
            result.browser=name;result.browserVersion=browser.version();result.failures??=[];result.passed??=[];
            result.failures.push(...errors);reports.push(result);
            console.log(`${name} ${result.browserVersion}: ${result.passed.length} passed, ${result.failures.length} failed, renderer ${result.renderer}, RGBA16F ${result.rgba16fRenderable}, context restores ${result.contextRestores}`);
            for(const failure of result.failures)console.error(`FAIL ${failure}`);
            for(const item of result.unverified??[])console.log(`UNVERIFIED ${item}`);
        } catch(error) {
            reports.push({browser:name,passed:[],failures:[String(error)],unverified:[]});
            console.error(`FAIL ${name}: ${error}`);
        } finally { if(browser)await browser.close(); }
    }
} finally {await new Promise(resolve=>server.close(resolve));}
if(options.report){fs.mkdirSync(path.dirname(path.resolve(options.report)),{recursive:true});fs.writeFileSync(options.report,JSON.stringify(reports,null,2)+"\n");}
if(reports.some(report=>report.failures.length))process.exitCode=1;
