const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { zipSync, unzipSync } = require('../webui/node_modules/fflate');
const { getProfile } = require('./profiles.cjs');
const root = path.resolve(__dirname, '..');
const digest = value => crypto.createHash('sha256').update(value).digest('hex');
async function main() {
  const profile = getProfile();
  if (profile.id !== 'android9') throw Error('独立 SH 包使用 --android9 构建');
  const source = unzipSync(await fs.readFile(path.join(profile.release, profile.filename)));
  const entries = Object.fromEntries(Object.entries(source).filter(([name]) => !['customize.sh', 'module.prop', 'service.sh', 'uninstall.sh', 'MANIFEST.json'].includes(name)));
  entries['fhm.sh'] = Buffer.from((await fs.readFile(path.join(root, 'standalone/fhm.sh'), 'utf8')).replaceAll('\r\n', '\n'));
  entries['README.md'] = await fs.readFile(path.join(root, 'STANDALONE.md'));
  const payload = zipSync(entries, { level: 6 });
  const installer = (await fs.readFile(path.join(root, 'standalone/install.sh'), 'utf8')).replaceAll('\r\n', '\n').replace('__PAYLOAD_SHA256__', digest(payload));
  const encoded = Buffer.from(payload).toString('base64').match(/.{1,76}/g).join('\n');
  const output = Buffer.from(installer + encoded + '\n');
  const filename = `fenghuangming-agent-android9-arm64-${profile.version}.sh`;
  await fs.writeFile(path.join(profile.release, filename), output);
  await fs.writeFile(path.join(profile.release, filename + '.sha256'), digest(output) + '  ' + filename + '\n');
  console.log(JSON.stringify({ filename, bytes: output.length, sha256: digest(output), payloadSha256: digest(payload) }));
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
