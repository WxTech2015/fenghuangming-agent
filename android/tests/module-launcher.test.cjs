const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const { execFileSync } = require('node:child_process');

test('安装失败后仍启动守护并保留诊断', async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'fhm-launcher-'));
  const shell = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : '/bin/sh';
  const unix = value => value.replaceAll('\\', '/').replace(/^([A-Za-z]):\//, (_, drive) => `/${drive.toLowerCase()}/`);
  try {
    const module = path.join(root, 'module'), data = path.join(root, 'data');
    for (const directory of [data, path.join(module, 'scripts'), path.join(module, 'bin'), path.join(root, 'bin')]) await fs.mkdir(directory, { recursive: true });
    const source = await fs.readFile(path.join(__dirname, '../module/scripts/manage.sh'), 'utf8');
    await fs.writeFile(path.join(module, 'scripts/manage.sh'), source.replaceAll('\r\n', '\n').replace('DATA=/data/adb/fenghuangming', `DATA='${unix(data)}'`));
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
