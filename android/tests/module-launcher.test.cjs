const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { execFileSync, spawn } = require('node:child_process');

test('安装失败后仍启动守护并保留诊断', async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'fhm-launcher-'));
  const shell = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : '/bin/sh';
  const unix = value => value.replaceAll('\\', '/').replace(/^([A-Za-z]):\//, (_, drive) => `/${drive.toLowerCase()}/`);
  try {
    const module = path.join(root, 'module'), data = path.join(root, 'data');
    for (const directory of [data, path.join(module, 'scripts'), path.join(module, 'bin'), path.join(root, 'bin')]) await fs.mkdir(directory, { recursive: true });
    const source = await fs.readFile(path.join(__dirname, '../module/scripts/manage.sh'), 'utf8');
    await fs.writeFile(path.join(module, 'scripts/manage.sh'), source.replaceAll('\r\n', '\n').replace('DATA=/data/adb/fenghuangming', `DATA='${unix(data)}'`).replace('/data/adb/ksu/bin/busybox', `'${unix(path.join(root, 'bin/busybox'))}'`));
    await fs.writeFile(path.join(root, 'bin/busybox'), '#!/bin/sh\nshift 3\nexec /bin/sh "$@"\n', { mode: 0o755 });
    await fs.writeFile(path.join(root, 'bin/getprop'), '#!/bin/sh\nprintf "1\\n"\n', { mode: 0o755 });
    await fs.writeFile(path.join(root, 'bin/sleep'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
    await fs.writeFile(path.join(module, 'bin/fhm-agent'), `#!/bin/sh
case " $* " in
  *" --version "*) echo 'test-version'; exit 0;;
  *" --install-player "*) echo 'Failure [INSTALL_FAILED_TEST]' >&2; exit 1;;
  *" --stop-player "*) exit 0;;
esac
touch '${unix(path.join(data, 'daemon.started'))}' '${unix(path.join(data, 'service.stopped'))}'
`, { mode: 0o755 });
    const output = execFileSync(shell, ['-c', 'PATH="$1/bin:$PATH" /bin/sh "$1/module/scripts/manage.sh" supervise', 'fhm-test', unix(root)], { encoding: 'utf8', timeout: 10000, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
    await fs.access(path.join(data, 'daemon.started'));
    assert.match(output, /守护仍会启动/);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});

test('并发启动只运行一套监控，停止后可重新启动', { skip: process.platform === 'win32' }, async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'fhm-lock-'));
  const module = path.join(root, 'module'), data = path.join(root, 'data'), bin = path.join(root, 'bin');
  let child;
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
  try {
    for (const directory of [data, bin, path.join(module, 'scripts'), path.join(module, 'bin')]) await fs.mkdir(directory, { recursive: true });
    const source = (await fs.readFile(path.join(__dirname, '../module/scripts/manage.sh'), 'utf8'))
      .replaceAll('\r\n', '\n').replace('#!/system/bin/sh', '#!/bin/sh')
      .replace('DATA=/data/adb/fenghuangming', `DATA='${data}'`)
      .replace('/data/adb/ksu/bin/busybox', `'${bin}/busybox'`);
    const manage = path.join(module, 'scripts/manage.sh');
    await fs.writeFile(manage, source, { mode: 0o755 });
    await fs.writeFile(path.join(bin, 'busybox'), '#!/bin/sh\nshift\nexec flock "$@"\n', { mode: 0o755 });
    await fs.writeFile(path.join(bin, 'getprop'), '#!/bin/sh\necho 1\n', { mode: 0o755 });
    await fs.writeFile(path.join(module, 'bin/fhm-agent'), `#!/bin/sh
case " $* " in
  *" --version "*) echo test; exit 0;;
  *" --install-player "*|*" --stop-player "*) exit 0;;
esac
echo started >> '${data}/starts'
while [ ! -f '${data}/release' ]; do sleep 0.05; done
touch '${data}/service.stopped'
`, { mode: 0o755 });
    const env = { ...process.env, PATH: `${bin}:${process.env.PATH}` };
    child = spawn('/bin/sh', [manage, 'supervise'], { env, stdio: 'ignore' });
    const closed = new Promise(resolve => child.once('close', resolve));
    for (let i = 0; i < 100 && !(await fs.stat(path.join(data, 'starts')).catch(() => null)); i++) await sleep(20);
    assert.equal((await fs.readFile(path.join(data, 'starts'), 'utf8')).trim(), 'started');
    assert.throws(() => execFileSync('/bin/sh', [manage, 'supervise'], { env, timeout: 1000, stdio: 'ignore' }));
    assert.equal((await fs.readFile(path.join(data, 'starts'), 'utf8')).trim(), 'started');
    await fs.writeFile(path.join(data, 'release'), '');
    await Promise.race([closed, sleep(5000).then(() => { throw new Error('监控未退出'); })]);
    await fs.rm(path.join(data, 'service.stopped'));
    execFileSync('/bin/sh', [manage, 'supervise'], { env, timeout: 5000, stdio: 'ignore' });
    assert.equal((await fs.readFile(path.join(data, 'starts'), 'utf8')).trim().split('\n').length, 2);
  } finally {
    if (child && child.exitCode === null) child.kill();
    await fs.rm(root, { recursive: true, force: true });
  }
});
