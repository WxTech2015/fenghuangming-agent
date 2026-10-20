const fs = require('node:fs/promises');
const path = require('node:path');
const { randomBytes } = require('node:crypto');
const { spawn } = require('node:child_process');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const { getProfile } = require('./profiles.cjs');
function run(command, args, env, shell = false) { return new Promise((ok, fail) => {
  const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', windowsHide: true, shell, windowsVerbatimArguments: path.basename(command).toLowerCase() === 'cmd.exe' });
  child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(new Error(`Android 构建退出 ${code}`)));
}); }
async function main() {
  const profile = getProfile();
  const tools = JSON.parse(await fs.readFile(path.join(root, '.tools/paths.json'), 'utf8'));
  const signing = path.join(root, '.signing'); await fs.mkdir(signing, { recursive: true });
  const passwordFile = path.join(signing, 'password.txt');
  let password; try { password = await fs.readFile(passwordFile, 'utf8'); } catch { password = randomBytes(32).toString('hex'); await fs.writeFile(passwordFile, password, { mode: 0o600, flag: 'wx' }); }
  const keystore = path.join(signing, 'player.jks');
  const androidUser = path.join(root, '.tools/android-user'); await fs.mkdir(androidUser, { recursive: true });
  const env = { ...process.env, JAVA_HOME: tools.java, PATH: path.join(tools.java, 'bin') + path.delimiter + process.env.PATH,
    ANDROID_USER_HOME: androidUser,
    ANDROID_HOME: process.env.ANDROID_HOME || path.join(process.env.LOCALAPPDATA, 'Android/Sdk'),
    GRADLE_USER_HOME: path.join(root, '.gradle'), FHM_KEYSTORE: keystore, FHM_STORE_PASSWORD: password };
  if (!(await fs.stat(keystore).catch(() => null))) await run(path.join(tools.java, 'bin/keytool.exe'), ['-genkeypair', '-keystore', keystore,
    '-storepass:env', 'FHM_STORE_PASSWORD', '-keypass:env', 'FHM_STORE_PASSWORD', '-alias', 'fenghuangming', '-keyalg', 'RSA', '-keysize', '3072',
    '-validity', '10000', '-dname', 'CN=Fenghuangming, O=Qingzhou Zhengchen, C=CN'], env);
  const gradleCommand = `""${tools.gradle}" --no-daemon -PfhmMinApi=${profile.minApi} -PfhmVersionCode=${profile.versionCode} -PfhmVersion=${profile.version} :player:testReleaseUnitTest :player:lintRelease :player:assembleRelease"`;
  await run(process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', gradleCommand], env);
  const javap = path.join(tools.java, 'bin/javap.exe');
  const service = execFileSync(javap, ['-classpath', path.join(root, 'player/build/tmp/kotlin-classes/release'), 'com.phoring.fenghuangming.player.PlaybackService'], { encoding: 'utf8', windowsHide: true });
  if (!service.includes('extends androidx.media3.session.MediaSessionService') || !service.includes('public com.phoring.fenghuangming.player.PlaybackService();')) throw new Error('播放服务字节码类型或构造函数无效');
  async function findJar(directory) {
    for (const item of await fs.readdir(directory, { withFileTypes: true })) {
      const filename = path.join(directory, item.name);
      if (item.isDirectory()) { const found = await findJar(filename); if (found) return found; }
      else if (item.name === 'media3-session-1.8.1-api.jar') return filename;
    }
  }
  const mediaJar = await findJar(path.join(root, '.gradle/caches/8.13/transforms'));
  if (!mediaJar) throw new Error('缺少 Media3 字节码检查依赖');
  const parent = execFileSync(javap, ['-classpath', mediaJar, 'androidx.media3.session.MediaSessionService'], { encoding: 'utf8', windowsHide: true });
  if (!parent.includes('extends android.app.Service')) throw new Error('Media3 服务不继承 Android Service');
  const apk = path.join(root, 'player/build/outputs/apk/release/player-release.apk');
  const aapt = path.join(env.ANDROID_HOME, 'build-tools/36.1.0/aapt2.exe');
  const metadata = execFileSync(aapt, ['dump', 'badging', apk], { encoding: 'utf8', windowsHide: true });
  if (!metadata.includes(`minSdkVersion:'${profile.minApi}'`) || !metadata.includes(`versionName='${profile.version}'`) || !metadata.includes(`versionCode='${profile.versionCode}'`)) throw new Error('APK 构建配置与目标版本不一致');
  await fs.mkdir(profile.build, { recursive: true });
  await fs.copyFile(apk, profile.apk);
  console.log(`已保存 ${profile.id} APK，Android ${profile.android}+。`);
  console.log('真实字节码继承与公开无参构造函数检查通过。');
  console.log('原生播放 APK 与单元测试完成。签名私钥保存在 android/.signing，不得上传。');
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
