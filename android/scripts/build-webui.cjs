const path = require('node:path');
const { spawn } = require('node:child_process');
const { getProfile } = require('./profiles.cjs');
const root = path.resolve(__dirname, '../webui');
async function run(file, args, env) {
  await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [path.join(root, 'node_modules', file), ...args], { cwd: root, env, stdio: 'inherit', windowsHide: true });
    child.once('error', reject); child.once('exit', code => code === 0 ? resolve() : reject(new Error(`WebUI 构建退出 ${code}`)));
  });
}
async function main() {
  const profile = getProfile();
  const env = { ...process.env, VITE_AGENT_VERSION: profile.version };
  await run('vue-tsc/bin/vue-tsc.js', ['--noEmit'], env);
  await run('vite/bin/vite.js', ['build', '--target', profile.id === 'android9' ? 'chrome66' : 'es2018', '--outDir', profile.webroot], env);
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
