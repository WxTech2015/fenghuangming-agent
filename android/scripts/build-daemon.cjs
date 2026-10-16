const fs = require('node:fs/promises');
const path = require('node:path');
const { spawn } = require('node:child_process');
const root = path.resolve(__dirname, '..');
async function main() {
  const tools = JSON.parse(await fs.readFile(path.join(root, '.tools/paths.json'), 'utf8'));
  const compiler = path.join(root, '.tools/android-ndk-r27d/toolchains/llvm/prebuilt/windows-x86_64/bin/clang.exe');
  await fs.access(compiler);
  const env = { ...process.env, GOMODCACHE: path.join(root, '.tools/gomod'), GOCACHE: path.join(root, '.tools/gocache'),
    GOOS: 'android', GOARCH: 'arm64', CGO_ENABLED: '1', CC: `"${compiler}" --target=aarch64-linux-android29` };
  const output = path.join(root, '../release/android/fhm-agent'); await fs.mkdir(path.dirname(output), { recursive: true });
  await new Promise((ok, fail) => {
    const child = spawn(tools.go, ['build', '-buildvcs=false', '-buildmode=pie', '-trimpath', '-ldflags=-s -w', '-o', output, './'], { cwd: path.join(root, 'daemon'), env, stdio: 'inherit', windowsHide: true });
    child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(new Error(`Android 守护编译退出 ${code}`)));
  });
  const binary = await fs.readFile(output);
  if (binary.readUInt16LE(18) !== 183 || !binary.includes(Buffer.from('/system/bin/linker64'))) throw new Error('守护程序不是 Android ARM64 PIE');
  console.log('Android ARM64 守护编译完成，使用 Bionic 与系统 DNS。');
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
