const fs = require('node:fs/promises');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { zipSync, unzipSync } = require('../webui/node_modules/fflate');
const root = path.resolve(__dirname, '..');
const release = path.resolve(root, '../release/android');
const version = '0.2.0-android.2';
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
async function walk(directory, prefix = '') {
  const files = [];
  for (const entry of await fs.readdir(directory, { withFileTypes: true })) {
    const filename = path.join(directory, entry.name), name = prefix + entry.name;
    if (entry.isSymbolicLink()) throw new Error('模块文件不接受符号链接');
    if (entry.isDirectory()) files.push(...await walk(filename, name + '/'));
    else files.push([name, await fs.readFile(filename)]);
  }
  return files;
}
async function main() {
  const entries = {};
  for (const [name, content] of await walk(path.join(root, 'module'))) {
    entries[name] = /\.(sh|prop)$/.test(name) ? Buffer.from(content.toString('utf8').replace(/^\uFEFF/, '').replace(/\r\n/g, '\n')) : content;
  }
  const apk = await fs.readFile(path.join(root, 'player/build/outputs/apk/release/player-release.apk'));
  entries['assets/player.apk'] = apk;
  entries['assets/player.sha256'] = Buffer.from(digest(apk) + '\n');
  const binary = await fs.readFile(path.join(release, 'fhm-agent'));
  if (!binary.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46])) || binary.readUInt16LE(18) !== 183) throw new Error('守护程序不是 ARM64 ELF');
  if (!binary.includes(Buffer.from('/system/bin/linker64'))) throw new Error('守护程序未使用 Android 链接器');
  entries['bin/fhm-agent'] = binary;
  if (binary.readUInt16LE(16) !== 3) throw new Error('Android 守护程序必须使用 PIE');
  for (const [name, content] of await walk(path.join(root, 'build/webroot'))) entries['webroot/' + name] = content;
  entries['LICENSE'] = await fs.readFile(path.join(root, '../LICENSE'));
  entries['README.md'] = await fs.readFile(path.join(root, 'README.md'));
  entries['THIRD_PARTY.md'] = await fs.readFile(path.join(root, 'THIRD_PARTY.md'));
  entries['VERIFICATION.md'] = await fs.readFile(path.join(root, 'VERIFICATION.md'));
  entries['licenses/go.txt'] = await fs.readFile(path.join(root, '.tools/go/go/LICENSE'));
  entries['licenses/gorilla-websocket.txt'] = await fs.readFile(path.join(root, '.tools/gomod/github.com/gorilla/websocket@v1.5.3/LICENSE'));
  const lock = JSON.parse(await fs.readFile(path.join(root, 'webui/package-lock.json'), 'utf8'));
  for (const relative of Object.keys(lock.packages).filter(name => name.startsWith('node_modules/'))) {
    const directory = path.join(root, 'webui', relative);
    for (const name of await fs.readdir(directory).catch(() => [])) {
      if (/^(?:license|copying|notice)(?:\..*)?$/i.test(name) && (await fs.stat(path.join(directory, name))).isFile()) {
        entries['licenses/npm/' + relative.slice(13).replaceAll('/', '_') + '/' + name] = await fs.readFile(path.join(directory, name));
      }
    }
  }
  const apache = await fetch('https://www.apache.org/licenses/LICENSE-2.0.txt', { signal: AbortSignal.timeout(30000) });
  if (!apache.ok) throw new Error('Apache 许可证下载失败');
  const apacheText = await apache.text();
  if (!apacheText.includes('Apache License') || !apacheText.includes('Version 2.0')) throw new Error('Apache 许可证内容无效');
  entries['licenses/Apache-2.0.txt'] = Buffer.from(apacheText);
  entries['licenses/NOTICE.txt'] = Buffer.from('AndroidX Media3: Copyright The Android Open Source Project.\nKotlin: Copyright JetBrains s.r.o. and Kotlin Programming Language contributors.\n');
  entries['MANIFEST.json'] = Buffer.from(JSON.stringify({ version, status: 'preview-not-device-validated', files: Object.entries(entries).map(([name, content]) => ({ path: name, bytes: content.length, sha256: digest(content) })) }, null, 2));
  const privateFile = Object.keys(entries).find(name => /\.(?:jks|keystore)$|(?:^|\/)password[^/]*$|(?:^|\/)config\.json$|(?:^|\/)local\.properties$|(?:^|\/)(?:\.signing|node_modules)(?:\/|$)/i.test(name));
  if (privateFile) throw new Error('模块包混入私有配置或构建依赖：' + privateFile);
  const zipped = zipSync(entries, { level: 6 });
  const checked = unzipSync(zipped);
  for (const [name, content] of Object.entries(entries)) if (!Buffer.from(checked[name] || []).equals(content)) throw new Error('ZIP 完整性不一致');
  await fs.mkdir(release, { recursive: true });
  const filename = `fenghuangming-agent-sukisu-arm64-${version}.zip`;
  await fs.writeFile(path.join(release, filename), zipped);
  await fs.writeFile(path.join(release, filename + '.sha256'), digest(zipped) + '  ' + filename + '\n');
  console.log(JSON.stringify({ filename, bytes: zipped.length, sha256: digest(zipped), files: Object.keys(entries).length }));
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
