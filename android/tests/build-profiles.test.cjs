const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { getProfile, moduleFile } = require('../scripts/profiles.cjs');
const unix = value => value.replaceAll('\\', '/').replace(/^([A-Za-z]):\//, (_, drive) => `/${drive.toLowerCase()}/`);
async function install(profile, api) {
  const base = path.resolve(__dirname, '../.tools/profile-tests');
  await fs.mkdir(base, { recursive: true });
  const directory = await fs.mkdtemp(path.join(base, 'api-'));
  const shell = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : '/bin/sh';
  try {
    await fs.mkdir(path.join(directory, 'module'));
    const source = moduleFile('customize.sh', await fs.readFile(path.join(__dirname, '../module/customize.sh')), profile)
      .toString().replace('DATA=/data/adb/fenghuangming', `DATA='${unix(path.join(directory, 'data'))}'`);
    await fs.writeFile(path.join(directory, 'customize.sh'), source);
    const script = 'abort(){ echo "$1" >&2; exit 1; }; set_perm(){ :; }; ui_print(){ :; }; MODPATH="$1/module"; . "$1/customize.sh"';
    execFileSync(shell, ['-c', script, 'test', unix(directory)], { env: { ...process.env, KSU: 'true', ARCH: 'arm64', API: String(api) }, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    assert.deepEqual(JSON.parse(await fs.readFile(path.join(directory, 'data/control.json'), 'utf8')), { blocked: true });
    await fs.access(path.join(directory, 'module/skip_mount'));
  } finally {
    assert.ok(path.resolve(directory).startsWith(base + path.sep));
    await fs.rm(directory, { recursive: true, force: true });
  }
}
test('Android 9 包允许 API 28 并保留初始制动，常规包仍拒绝 API 28', async () => {
  await install(getProfile(['--android9']), 28);
  await assert.rejects(install(getProfile([]), 28), /Android 10/);
  await assert.rejects(install(getProfile(['--android9']), 27), /Android 9/);
  await install(getProfile([]), 29);
});
test('两种构建产物隔离，不接受未知参数', () => {
  assert.notEqual(getProfile([]).release, getProfile(['--android9']).release);
  assert.notEqual(getProfile([]).apk, getProfile(['--android9']).apk);
  assert.notEqual(getProfile([]).webroot, getProfile(['--android9']).webroot);
  assert.throws(() => getProfile(['--androdi9']));
});
