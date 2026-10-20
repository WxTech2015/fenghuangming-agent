const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { WebSocketServer } = require('../../node_modules/ws');
const root = path.resolve(__dirname, '../..');
const options = new Map();
for (let i = 2; i < process.argv.length; i += 2)
    options.set(process.argv[i], process.argv[i + 1]);
const adb = options.get('--adb') || 'adb', serial = options.get('--serial'), binary = options.get('--daemon');
const apk = path.resolve(options.get('--apk') || path.join(root, 'android/player/build/outputs/apk/release/player-release.apk'));
if (!serial || !binary) {
    console.error('仅在隔离模拟器运行：node android/tests/emulator-smoke.cjs --serial <ADB设备> --daemon <同架构守护程序> [--adb <adb路径>]');
    process.exit(2);
}
const pkg = 'com.phoring.fenghuangming.player';
const dir = '/data/local/tmp/fhm-emulator-smoke-' + crypto.randomBytes(4).toString('hex');
const port = Number(options.get('--port') || 5193);
const token = 'emulator-smoke-only-token';
const report = { steps: [], messages: [] };
function call(...args) { return execFileSync(adb, ['-s', serial, ...args], { encoding: 'utf8', timeout: 20000, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] }).trim(); }
function shell(text) {
    const quoted = "'" + text.replaceAll("'", "'\\''") + "'";
    return options.get('--root-mode') === 'adb' ? call('shell', 'sh -c ' + quoted) : call('shell', 'su', '-c', quoted);
}
function ctl(value) { return JSON.parse(shell(dir + '/fhm-agent --data ' + dir + '/data --request ' + Buffer.from(JSON.stringify(value)).toString('base64'))); }
const sleep = ms => new Promise(r => setTimeout(r, ms));
async function until(label, predicate, timeout = 15000) { const start = Date.now(); while (Date.now() - start < timeout) {
    if (await predicate()) {
        console.log('PASS ' + label);
        report.steps.push(label);
        return;
    }
    await sleep(200);
} throw Error('TIMEOUT ' + label + ' ' + JSON.stringify(ctl({ action: 'state' }))); }
function wav() { const rate = 48000, seconds = 35, bytes = rate * seconds * 2, b = Buffer.alloc(bytes + 44); b.write('RIFF'); b.writeUInt32LE(bytes + 36, 4); b.write('WAVEfmt ', 8); b.writeUInt32LE(16, 16); b.writeUInt16LE(1, 20); b.writeUInt16LE(1, 22); b.writeUInt32LE(rate, 24); b.writeUInt32LE(rate * 2, 28); b.writeUInt16LE(2, 32); b.writeUInt16LE(16, 34); b.write('data', 36); b.writeUInt32LE(bytes, 40); for (let i = 0; i < rate * seconds; i++)
    b.writeInt16LE(Math.round(120 * Math.sin(2 * Math.PI * 440 * i / rate)), 44 + i * 2); return b; }
const audio = wav(), digest = crypto.createHash('sha256').update(audio).digest('hex');
let ws, leaseTimer, daemonPid, server, renew = true, acceptConnections = true, originalWifi;
let commandId = 0;
function command(payload) { ws.send(JSON.stringify({ v: 1, kind: 'command', id: 'smoke_' + (++commandId), agentId: 'emulator', zoneId: 'test', epoch: 1, expiresAt: new Date(Date.now() + 30000).toISOString(), payload })); }
async function main() {
    report.android = call('shell', 'getprop', 'ro.build.version.release');
    report.abi = call('shell', 'getprop', 'ro.product.cpu.abilist');
    report.selinux = call('shell', 'getenforce');
    if (options.get('--root-mode') === 'adb' && call('shell', 'id', '-u') !== '0') throw Error('该模式仅用于 adb root 的隔离测试镜像');
    if (!report.abi.split(',').includes('x86_64'))
        throw Error('该脚本只用于隔离 x86_64 Android 模拟器');
    server = http.createServer((req, res) => { if (req.headers.authorization !== 'Bearer ' + token) {
        res.writeHead(401);
        res.end();
        return;
    } const range = /bytes=(\d+)-(\d+)/.exec(req.headers.range || ''); if (range) {
        const start = Number(range[1]), end = Number(range[2]);
        res.writeHead(206, { 'Content-Type': 'audio/wav', 'Content-Length': end - start + 1, 'Content-Range': `bytes ${start}-${end}/${audio.length}` });
        res.end(audio.subarray(start, end + 1));
    }
    else {
        res.writeHead(200, { 'Content-Type': 'audio/wav', 'Content-Length': audio.length });
        res.end(audio);
    } });
    const wss = new WebSocketServer({ server, path: '/ws/agents' });
    wss.on('connection', (socket, req) => { if (!acceptConnections || req.headers.authorization !== 'Bearer ' + token) {
        socket.close();
        return;
    } ws = socket; socket.on('message', raw => { const value = JSON.parse(raw); report.messages.push(value); if (value.type === 'hello')
        socket.send(JSON.stringify({ v: 1, type: 'welcome', agentId: 'emulator', zoneId: 'test', epoch: 1, leaseMs: 5000 })); if (value.type === 'playback')
        socket.send(JSON.stringify({ type: 'event-ack', eventId: value.eventId })); }); });
    await new Promise(r => server.listen(port, '127.0.0.1', r));
    leaseTimer = setInterval(() => { if (renew && ws?.readyState === 1)
        ws.send(JSON.stringify({ type: 'lease', epoch: 1, leaseMs: 5000 })); }, 1000);
    call('reverse', 'tcp:' + port, 'tcp:' + port);
    shell('mkdir -p ' + dir + '/module/assets ' + dir + '/data');
    shell('chmod 755 ' + dir);
    shell('chown 2000:2000 ' + dir + ' ' + dir + '/module/assets');
    shell('chmod 700 ' + dir + '/data');
    call('push', path.resolve(binary), dir + '/fhm-agent');
    call('push', apk, dir + '/module/assets/player.apk');
    shell('chmod 755 ' + dir + '/fhm-agent');
    shell('chown 0:0 ' + dir + ' ' + dir + '/module/assets');
    shell('chmod 700 ' + dir);
    const apkHash = crypto.createHash('sha256').update(fs.readFileSync(apk)).digest('hex');
    shell("echo '" + apkHash + "' > " + dir + '/module/assets/player.sha256');
    console.log(shell(dir + '/fhm-agent --data ' + dir + '/data --module ' + dir + '/module --install-player'));
    const config = Buffer.from(JSON.stringify({ serverUrl: 'http://127.0.0.1:' + port, token, cacheMaxMb: 100, connections: 4 })).toString('base64');
    shell("echo '" + config + "' | base64 -d > " + dir + '/data/config.json');
    daemonPid = shell('nohup ' + dir + '/fhm-agent --data ' + dir + '/data --module ' + dir + '/module > ' + dir + '/daemon.log 2>&1 </dev/null & echo $!');
    await until('backend authentication', () => ctl({ action: 'state' }).authorized);
    ctl({ action: 'activate' });
    ctl({ action: 'release' });
    await until('native player ready', () => ctl({ action: 'state' }).playerReady);
    const statusBefore = JSON.parse(shell('cat /data/user/0/' + pkg + '/files/player-status.json'));
    if (!statusBefore.ready)
        throw Error('native startup status is not ready');
    report.steps.push('native startup screen receives ready state');
    command({ action: 'start', playbackId: 'smoke_play', queueItemId: 'test', assetId: 'test', ticket: 'smoke-ticket', sha256: digest, bytes: audio.length, mime: 'audio/wav', volume: 5, title: '模拟器播放验证', artists: ['本地合成测试音频'] });
    await until('native playback started', () => report.messages.some(v => v.type === 'playback' && v.playbackId === 'smoke_play' && v.state === 'started'));
    await until('playback position advances', () => ctl({ action: 'state' }).player?.position >= 2);
    let state = ctl({ action: 'state' });
    report.playing = state;
    console.log(JSON.stringify({ pause: ctl({ action: 'pause', playbackId: 'smoke_play' }) }));
    await until('pause', () => ctl({ action: 'state' }).player?.paused === true);
    console.log(JSON.stringify({ seek: ctl({ action: 'seek', playbackId: 'smoke_play', value: 7 }) }));
    await until('seek to exact 7 seconds while paused', () => { const p = ctl({ action: 'state' }).player; return p?.paused && Math.abs(p.position - 7) < 0.1; });
    ctl({ action: 'seek', playbackId: 'smoke_play', value: 8 });
    await until('one second seek', () => Math.abs(ctl({ action: 'state' }).player?.position - 8) < 0.1);
    ctl({ action: 'resume', playbackId: 'smoke_play' });
    await until('resume', () => ctl({ action: 'state' }).player?.position >= 9);
    const start = Date.now();
    ctl({ action: 'brake' });
    report.brakeMillis = Date.now() - start;
    await until('brake persists and playback stops', () => { const s = ctl({ action: 'state' }); return s.emergencyStopped && !s.playerReady && !s.player?.playbackId; });
    ctl({ action: 'activate' });
    await sleep(1500);
    if (!ctl({ action: 'state' }).emergencyStopped)
        throw Error('brake unexpectedly released');
    report.steps.push('activation does not release brake');
    ctl({ action: 'release' });
    await until('player reconnects after brake', () => ctl({ action: 'state' }).playerReady);
    command({ action: 'start', playbackId: 'lease_play', queueItemId: 'test2', assetId: 'test2', ticket: 'smoke-ticket', sha256: digest, bytes: audio.length, mime: 'audio/wav', volume: 5, title: '租约验证', artists: ['本地合成测试音频'] });
    await until('second native playback started', () => report.messages.some(v => v.type === 'playback' && v.playbackId === 'lease_play' && v.state === 'started'));
    renew = false;
    await until('expired lease stops playback', () => { const p = ctl({ action: 'state' }).player; return !p?.playbackId; }, 10000);
    if (options.get('--network-recovery') === 'yes') {
        if (call('shell', 'getprop', 'ro.kernel.qemu') !== '1') throw Error('断网恢复测试只允许隔离模拟器');
        originalWifi = shell('settings get global wifi_on');
        report.networkRecovery = { recoveries: [] };
        ctl({ action: 'brake' });
        const before = ctl({ action: 'state' });
        const initialPlayer = before.diagnostics.playerPid;
        const daemon = before.diagnostics.daemonPid;
        shell('svc wifi disable');
        await until('Wi-Fi is actually disabled', () => shell('settings get global wifi_on') === '0');
        // ADB reverse 不随 Wi-Fi 断开，额外拒绝测试后端连接以模拟真实网络中断。
        acceptConnections = false;
        ws.terminate();
        await until('backend offline with native standby alive', () => {
            const s = ctl({ action: 'state' });
            return !s.authorized && s.playerConnected && s.player.cpuAwake && s.diagnostics.daemonPid === daemon;
        });
        await sleep(6000);
        const offline = ctl({ action: 'state' });
        if (offline.authorized || !offline.playerConnected || offline.diagnostics.playerPid !== initialPlayer) throw Error('断网导致原生待机退出');
        report.steps.push('native PID survives Wi-Fi disconnect and expired backend lease');
        for (let round = 1; round <= 3; round++) {
            const pid = ctl({ action: 'state' }).diagnostics.playerPid;
            const start = Date.now();
            shell('am force-stop --user 0 ' + pkg);
            await until('offline native recovery ' + round, () => {
                const s = ctl({ action: 'state' });
                return !s.authorized && s.playerConnected && s.player.ready && s.player.cpuAwake && s.emergencyStopped &&
                    s.diagnostics.playerPid !== pid && s.diagnostics.daemonPid === daemon;
            }, 12000);
            const s = ctl({ action: 'state' });
            report.networkRecovery.recoveries.push({ millis: Date.now() - start, before: pid, after: s.diagnostics.playerPid });
            const adj = Number(shell('cat /proc/' + s.diagnostics.playerPid + '/oom_score_adj'));
            if (adj > -700 || s.diagnostics.keepaliveError) throw Error('Root 播放进程保护没有生效');
        }
        acceptConnections = true;
        renew = true;
        shell('svc wifi enable');
        await until('network returns without manual activation', () => {
            const s = ctl({ action: 'state' });
            return s.authorized && s.playerConnected && s.emergencyStopped && s.diagnostics.daemonPid === daemon;
        });
        report.networkRecovery.final = ctl({ action: 'state' });
    }
    report.crash = call('logcat', '-d', '-b', 'crash', '-t', '120');
    if (report.crash.includes(pkg))
        throw Error('player crash buffer is not empty');
    report.steps.push('no player crash');
    report.success = true;
}
main().catch(error => { report.success = false; report.error = error.message; console.error(error); try {
    report.logs = shell('cat ' + dir + '/daemon.log');
    report.crash = call('logcat', '-d', '-b', 'crash', '-t', '120');
    console.error(report.logs, report.crash);
}
catch { } }).finally(async () => { try {
    if (originalWifi !== undefined) shell('svc wifi ' + (originalWifi === '0' ? 'disable' : 'enable'));
    ctl({ action: 'brake' });
    if (/^\d+$/.test(daemonPid || '') && shell('readlink /proc/' + daemonPid + '/exe') === dir + '/fhm-agent')
        shell('kill -TERM ' + daemonPid);
    shell('/system/bin/am force-stop --user 0 ' + pkg);
    call('reverse', '--remove', 'tcp:' + port);
}
catch { } await sleep(200); if (/^\/data\/local\/tmp\/fhm-emulator-smoke-[a-f0-9]{8}$/.test(dir)) {
    try {
        shell('rm -rf -- ' + dir);
    }
    catch { }
} clearInterval(leaseTimer); ws?.terminate(); server?.close(); fs.mkdirSync(path.join(root, 'android/.tools'), { recursive: true }); fs.writeFileSync(path.join(root, 'android/.tools/emulator-smoke-report.json'), JSON.stringify(report, null, 2)); console.log(JSON.stringify({ success: report.success, steps: report.steps, brakeMillis: report.brakeMillis, error: report.error })); process.exitCode = report.success ? 0 : 1; });
