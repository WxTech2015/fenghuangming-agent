const fs = require('node:fs/promises');
const { createReadStream, createWriteStream } = require('node:fs');
const { createHash } = require('node:crypto');
const { pipeline } = require('node:stream/promises');
const { Readable } = require('node:stream');
const { spawn } = require('node:child_process');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const tools = path.join(root, '.tools');
async function json(url) { const response = await fetch(url); if (!response.ok) throw new Error(`工具元数据 HTTP ${response.status}`); return response.json(); }
async function hash(file) { const digest = createHash('sha256'); for await (const data of createReadStream(file)) digest.update(data); return digest.digest('hex'); }
async function download(url, file, expected) {
  try { if (await hash(file) === expected) return; } catch {}
  console.log(`下载 ${path.basename(file)}`);
  const response = await fetch(url, { signal: AbortSignal.timeout(300000) });
  if (!response.ok) throw new Error(`下载工具 HTTP ${response.status}`);
  await pipeline(Readable.fromWeb(response.body), createWriteStream(file + '.tmp'));
  if (await hash(file + '.tmp') !== expected) throw new Error('工具 SHA-256 不匹配');
  await fs.rename(file + '.tmp', file);
}
function run(command, args) { return new Promise((ok, fail) => { const child = spawn(command, args, { stdio: 'inherit', windowsHide: true }); child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(new Error(`工具退出 ${code}`))); }); }
async function extract(file, destination) { await fs.mkdir(destination, { recursive: true }); await run('tar.exe', ['-xf', file, '-C', destination]); }
async function main() {
  await fs.mkdir(tools, { recursive: true });
  const list = await json('https://go.dev/dl/?mode=json');
  const release = list.find(item => item.stable);
  const go = release.files.find(item => item.os === 'windows' && item.arch === 'amd64' && item.kind === 'archive');
  const java = (await json('https://api.adoptium.net/v3/assets/latest/21/hotspot?architecture=x64&image_type=jdk&os=windows&vendor=eclipse'))[0].binary.package;
  const gradleVersion = '8.13';
  const gradleUrl = `https://services.gradle.org/distributions/gradle-${gradleVersion}-bin.zip`;
  const digestResponse = await fetch(gradleUrl + '.sha256');
  if (!digestResponse.ok) throw new Error('Gradle 校验信息不可用');
  const gradleHash = (await digestResponse.text()).trim();
  const sources = [
    { key: 'go', url: `https://go.dev/dl/${go.filename}`, filename: go.filename, digest: go.sha256, folder: 'go' },
    { key: 'java', url: java.link, filename: java.name, digest: java.checksum, folder: 'jdk21' },
    { key: 'gradle', url: gradleUrl, filename: `gradle-${gradleVersion}-bin.zip`, digest: gradleHash, folder: 'gradle' },
  ];
  await Promise.all(sources.map(async item => {
    if (!/^[a-f0-9]{64}$/i.test(item.digest)) throw new Error('无效的上游校验信息');
    const file = path.join(tools, item.filename);
    await download(item.url, file, item.digest);
    const directory = path.join(tools, item.folder);
    const present = await fs.stat(directory).catch(() => null);
    if (!present) await extract(file, directory);
  }));
  const javaFolder = (await fs.readdir(path.join(tools, 'jdk21'), { withFileTypes: true })).find(item => item.isDirectory()).name;
  const config = { go: path.join(tools, 'go', 'go', 'bin', 'go.exe'), java: path.join(tools, 'jdk21', javaFolder), gradle: path.join(tools, 'gradle', `gradle-${gradleVersion}`, 'bin', 'gradle.bat') };
  await fs.writeFile(path.join(tools, 'paths.json'), JSON.stringify(config, null, 2));
  console.log('隔离构建工具准备完成。');
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
