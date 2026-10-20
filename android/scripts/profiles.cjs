const path = require('node:path');
const root = path.resolve(__dirname, '..');
const profiles = {
  modern: { id: 'modern', version: '0.2.0-android.7', versionCode: 20009, minApi: 29, android: '10', directory: '../release/android' },
  android9: { id: 'android9', version: '0.2.0-android9.3', versionCode: 20010, minApi: 28, android: '9', directory: '../release/android/android9' },
};
function getProfile(args = process.argv.slice(2)) {
  const unknown = args.find(value => value !== '--android9');
  if (unknown) throw new Error('不支持的构建参数：' + unknown);
  const profile = profiles[args.includes('--android9') ? 'android9' : 'modern'];
  const build = path.join(root, 'build', profile.id);
  return { ...profile, build, apk: path.join(build, 'player-release.apk'), webroot: path.join(build, 'webroot'),
    release: path.resolve(root, profile.directory), filename: `fenghuangming-agent-sukisu-${profile.id === 'android9' ? 'android9-' : ''}arm64-${profile.version}.zip` };
}
function moduleFile(name, content, profile) {
  if (!/\.(sh|prop)$/.test(name)) return content;
  let text = content.toString('utf8').replace(/^\uFEFF/, '').replace(/\r\n/g, '\n');
  if (name === 'customize.sh') text = text.replace(/^MIN_API=\d+$/m, `MIN_API=${profile.minApi}`).replace(/^MIN_ANDROID=\d+$/m, `MIN_ANDROID=${profile.android}`);
  if (name === 'module.prop') {
    text = text.replace(/^version=.*$/m, `version=${profile.version}`).replace(/^versionCode=.*$/m, `versionCode=${profile.versionCode}`);
    if (profile.id === 'android9') text = text.replace(/^name=.*$/m, 'name=凤凰鸣 Android Agent · Android 9 兼容版')
      .replace(/^description=.*$/m, 'description=EMUI 9 / Android 9 ARM64 兼容播放端，适用于已正常运行的 SukiSU，保留原生播放、后台保活与模块 WebUI。');
  }
  return Buffer.from(text);
}
module.exports = { getProfile, moduleFile };
