<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { NButton, NCard, NConfigProvider, NInput, NInputNumber, NSlider, NSpace, NTabPane, NTabs, NTag, darkTheme } from 'naive-ui';
import { request, service } from './bridge';
const agentVersion = import.meta.env.VITE_AGENT_VERSION || '0.2.0-android.6';
type State = { endpoint?: string; hasToken?: boolean; authorized?: boolean; serverConnected?: boolean; daemonOffline?: boolean; playerConnected?: boolean; playerReady?: boolean; emergencyStopped?: boolean; playerError?: string; cacheMaxMb?: number; connections?: number; current?: { title?: string; artists?: string[]; playbackId?: string }; player?: { ready?: boolean; cpuAwake?: boolean; wifiLocked?: boolean; playbackId?: string; position?: number; durationSeconds?: number; paused?: boolean; seekable?: boolean; output?: string; systemVolume?: number }; transfer?: { bytes: number; total: number; speedBytesPerSecond: number }; overview?: { waitingCount?: number; queue?: { id: string; title: string; artists?: string[] }[] } };
const state = ref<State>({}); const logs = ref<string[]>([]); const error = ref(''); const message = ref(''); const busy = ref(false); const refreshing = ref(false);
const endpoint = ref(''); const token = ref(''); const cache = ref(512); const connections = ref(4); const second = ref(0); const volume = ref(50); const initialized = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;
const player = computed(() => state.value.player || {});
const diagnostics = computed(() => (state.value as State & { diagnostics?: { daemonPid?: number; playerPid?: number; uptimeSeconds?: number; playerConnections?: number; lastPlayerDisconnect?: string; keepaliveError?: string } }).diagnostics || {});
const maximum = computed(() => Math.max(0, Math.floor(player.value.durationSeconds || 0) - 1));
const canSeek = computed(() => state.value.playerReady && player.value.seekable && maximum.value > 0 && !state.value.emergencyStopped);
const time = (value = 0) => `${Math.floor(value / 60)}:${Math.floor(value % 60).toString().padStart(2, '0')}`;
async function refresh() {
  if (refreshing.value) return; refreshing.value = true;
  try {
    state.value = await request<State>({ action: 'state' });
    if (!initialized.value && !state.value.daemonOffline) { endpoint.value = state.value.endpoint || ''; cache.value = state.value.cacheMaxMb || 512; connections.value = state.value.connections || 4; initialized.value = true; }
    logs.value = (await request<{ lines: string[] }>({ action: 'logs' })).lines || [];
  } catch (e) { error.value = (e as Error).message; } finally { refreshing.value = false; }
}
async function act(payload: unknown, notice = '') {
  busy.value = true; error.value = ''; message.value = '';
  try { await request(payload); message.value = notice; await refresh(); } catch (e) { error.value = (e as Error).message; } finally { busy.value = false; }
}
async function manage(action: 'start' | 'stop' | 'install') {
  busy.value = true; error.value = '';
  try { await service(action); initialized.value = false; message.value = '已提交服务操作'; } catch (e) { error.value = (e as Error).message; } finally { busy.value = false; await refresh(); }
}
async function configure() {
  await act({ action: 'configure', config: { serverUrl: endpoint.value.trim(), token: token.value.trim(), cacheMaxMb: cache.value, connections: connections.value } }, '已保存，重新连接后请手动解除制动');
  token.value = '';
}
function seek(value: number) { return act({ action: 'seek', playbackId: player.value.playbackId, value: Math.round(Math.min(maximum.value, Math.max(0, value))) }); }
function controlTrack(action: string) { return act({ action, playbackId: player.value.playbackId || state.value.current?.playbackId }); }
function exportLogs() { const file = new Blob([logs.value.join('\n')], { type: 'text/plain;charset=utf-8' }); const url = URL.createObjectURL(file); const a = document.createElement('a'); a.href = url; a.download = 'fenghuangming-android.log'; a.click(); URL.revokeObjectURL(url); }
onMounted(() => { void refresh(); timer = setInterval(refresh, 2000); }); onUnmounted(() => { if (timer) clearInterval(timer); });
</script>

<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="{ common: { primaryColor: '#b6a7ff', primaryColorHover: '#cec3ff', borderRadius: '16px', cardColor: '#1b2032' } }">
    <main>
      <header><div class="mark">鳴</div><div><small>ANDROID AGENT</small><h1>凤凰鸣</h1><p>本机控制台</p></div><n-tag :type="state.authorized ? 'success' : 'warning'" round>{{ state.authorized ? '后端已连接' : state.daemonOffline ? '守护未运行' : '后端未连接' }}</n-tag></header>
      <section class="brake" :class="{ locked: state.emergencyStopped }"><div><strong>{{ state.emergencyStopped ? '紧急制动已锁定' : '本机紧急制动' }}</strong><p>{{ state.emergencyStopped ? '重启后仍保持；解除不会重播旧歌曲' : '立即停止播放，并禁止后续歌曲' }}</p></div><n-button v-if="state.emergencyStopped" type="warning" @click="act({action:'release'})">解除制动</n-button><n-button v-else type="error" @click="act({action:'brake'})">立即制动</n-button></section>
      <p v-if="error" class="notice error">{{ error }}</p><p v-if="message" class="notice">{{ message }}</p>
      <n-tabs type="segment" animated>
        <n-tab-pane name="overview" tab="总览">
          <div class="status-grid"><n-card size="small"><small>守护进程</small><strong :class="{ good: !state.daemonOffline }">{{ state.daemonOffline ? '未运行' : state.endpoint !== undefined ? '运行中' : '检测中' }}</strong></n-card><n-card size="small"><small>原生播放服务</small><strong :class="{ good: state.playerConnected && player.ready }">{{ state.playerConnected && player.ready ? (state.emergencyStopped ? '待机 · 已制动' : '就绪') : state.playerConnected ? '已连接，未就绪' : '未连接' }}</strong></n-card></div>
          <n-card title="正在播放" class="track"><div class="cover">♫</div><h2>{{ state.current?.title || '等待下一首' }}</h2><p>{{ state.current?.artists?.join(' / ') || '群内分享音乐即可点歌' }}</p><div class="progress"><span>{{ time(player.position) }}</span><span>{{ time(player.durationSeconds) }}</span></div><n-slider :value="Math.floor(player.position || 0)" :max="maximum" :step="1" :disabled="!canSeek" @update:value="seek" /></n-card>
          <n-card title="设备与下载"><p>可用音频输出：{{ player.output || '尚未获取' }}</p><p>后台保活：CPU {{ player.cpuAwake ? '已保持唤醒' : '未锁定' }} · Wi-Fi {{ player.wifiLocked ? '已锁定' : '未锁定' }}</p><p>守护 PID：{{ diagnostics.daemonPid ?? '—' }} · 播放器 PID：{{ diagnostics.playerPid ?? '—' }}</p><p>守护运行：{{ time(diagnostics.uptimeSeconds) }} · 播放器重连：{{ Math.max(0, (diagnostics.playerConnections || 0) - 1) }} 次</p><p v-if="diagnostics.lastPlayerDisconnect">最近播放器断开：{{ new Date(diagnostics.lastPlayerDisconnect).toLocaleString() }}</p><p v-if="!state.authorized && !state.daemonOffline">后端连接中断时仍监控原生播放器，网络恢复后自动重连。播放租约到期会停止当前歌曲。</p><p v-if="diagnostics.keepaliveError" class="error">保活设置：{{ diagnostics.keepaliveError }}</p><p>系统媒体音量：{{ player.systemVolume ?? '—' }}</p><p v-if="state.transfer">下载：{{ (state.transfer.bytes / 1048576).toFixed(1) }} / {{ (state.transfer.total / 1048576).toFixed(1) }} MB · {{ (state.transfer.speedBytesPerSecond / 1048576).toFixed(2) }} MB/s</p><p v-if="state.playerError" class="error">{{ state.playerError }}</p></n-card>
          <n-card :title="`待播放 · ${state.overview?.waitingCount || 0}`"><div v-for="item in state.overview?.queue || []" :key="item.id" class="queue-row"><strong>{{ item.title }}</strong><small>{{ item.artists?.join(' / ') }}</small></div><p v-if="!state.overview?.queue?.length">当前没有待播歌曲</p></n-card>
        </n-tab-pane>
        <n-tab-pane name="play" tab="播放"><n-card title="播放控制"><n-space><n-button :disabled="!state.playerReady || busy || !player.playbackId || state.emergencyStopped" @click="controlTrack(player.paused?'resume':'pause')">{{ player.paused ? '继续' : '暂停' }}</n-button><n-button :disabled="!player.playbackId && !state.current?.playbackId" @click="controlTrack('stop')">停止当前歌曲</n-button></n-space><label>进度 · 按秒调整</label><n-slider :value="Math.floor(player.position || 0)" :max="maximum" :step="1" :disabled="!canSeek" @update:value="seek" /><n-space><n-button :disabled="!canSeek" @click="seek(Math.floor(player.position || 0)-1)">−1 秒</n-button><n-button :disabled="!canSeek" @click="seek(Math.floor(player.position || 0)+1)">+1 秒</n-button></n-space><n-space class="jump"><n-input-number v-model:value="second" :min="0" :max="maximum" :precision="0" /><n-button :disabled="!canSeek" @click="seek(second)">跳转</n-button></n-space><label>播放器相对音量</label><n-slider v-model:value="volume" :min="0" :max="100" :step="1" :disabled="state.emergencyStopped || !state.playerReady" @update:value="v=>act({action:'volume',value:v})" /><p>不修改手机全局媒体音量</p></n-card></n-tab-pane>
        <n-tab-pane name="settings" tab="设置"><n-card title="连接配置"><label>服务器根地址</label><n-input v-model:value="endpoint" placeholder="https://music.example.com" /><label>设备凭证</label><n-input v-model:value="token" type="password" show-password-on="click" :placeholder="state.hasToken ? '已保存；留空保持原值' : '填写后台注册设备时生成的凭证'" /><label>缓存上限 / MB</label><n-input-number v-model:value="cache" :min="100" :max="10240" :precision="0" /><label>下载并发</label><n-input-number v-model:value="connections" :min="1" :max="4" :precision="0" /><n-button type="primary" block class="save" :loading="busy" @click="configure">保存并重新连接</n-button></n-card><n-card title="本机服务"><n-space><n-button @click="manage('start')">启动守护</n-button><n-button @click="act({action:'activate'},'已请求启动原生播放器')">启动播放器</n-button><n-button @click="manage('install')">检查 / 安装播放组件</n-button><n-button type="warning" @click="manage('stop')">停止服务</n-button></n-space><p>停用后不会自动恢复；点击启动守护重新启用。</p></n-card></n-tab-pane>
        <n-tab-pane name="logs" tab="日志"><n-card title="运行日志"><template #header-extra><n-button size="small" @click="exportLogs">导出</n-button></template><pre>{{ logs.join('\n') || '暂无日志' }}</pre></n-card></n-tab-pane>
      </n-tabs><footer>凤凰鸣 · Android {{ agentVersion }}</footer>
    </main>
  </n-config-provider>
</template>
