const fs = require('node:fs/promises');
const { createReadStream, createWriteStream } = require('node:fs');
const { createHash } = require('node:crypto');
const { pipeline } = require('node:stream/promises');
const { Readable } = require('node:stream');
const { spawn } = require('node:child_process');
const path = require('node:path');
const tools = path.resolve(__dirname, '../.tools');
const filename = path.join(tools, 'android-ndk-r27d-windows.zip');
const expected = '56607cbccd3642d4a1991f6bb3114a00f884f426';
async function hash(file) { const value = createHash('sha1'); for await (const bytes of createReadStream(file)) value.update(bytes); return value.digest('hex'); }
async function main() {
  const compiler = path.join(tools, 'android-ndk-r27d/toolchains/llvm/prebuilt/windows-x86_64/bin/clang.exe');
  if (await fs.stat(compiler).catch(() => null)) { console.log('Android NDK 已准备。'); return; }
  if (await hash(filename).catch(() => '') !== expected) {
    const response = await fetch('https://dl.google.com/android/repository/android-ndk-r27d-windows.zip', { signal: AbortSignal.timeout(600000) });
    if (!response.ok) throw new Error(`NDK 下载 HTTP ${response.status}`);
    const stream = Readable.fromWeb(response.body); let bytes = 0, last = 0;
    stream.on('data', chunk => { bytes += chunk.length; if (bytes - last >= 100 * 1048576) { console.log(`NDK 已下载 ${(bytes / 1048576).toFixed(0)} MB`); last = bytes; } });
    await pipeline(stream, createWriteStream(filename + '.tmp'));
    if (await hash(filename + '.tmp') !== expected) throw new Error('NDK 官方校验不一致');
    await fs.rename(filename + '.tmp', filename);
  }
  console.log('NDK 校验完成，正在解压。');
  await new Promise((ok, fail) => { const child = spawn('tar.exe', ['-xf', filename, '-C', tools], { stdio: 'inherit', windowsHide: true }); child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(new Error('NDK 解压失败'))); });
  console.log('Android NDK r27d 准备完成。');
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
