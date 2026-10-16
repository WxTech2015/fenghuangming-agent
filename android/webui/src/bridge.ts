const control = '/data/adb/modules/fenghuangming_agent/scripts/ctl.sh';
export function encodeRequest(value: unknown): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value));
  if (bytes.length > 16000) throw new Error('配置内容过大');
  return btoa(String.fromCharCode(...bytes));
}
async function command(text: string): Promise<string> {
  const host = window as unknown as { ksu?: { exec: (...args: unknown[]) => unknown }; [key: string]: unknown };
  if (!host.ksu) throw new Error('请在 SukiSU / KernelSU 模块 WebUI 中打开');
  // Java 桥接的方法 length 不可靠，同时兼容回调式与返回字符串的实现。
  return new Promise((resolve, reject) => {
    const callback = '__fhm_' + crypto.getRandomValues(new Uint32Array(2)).join('_');
    const finish = (error: Error | null, output = '') => { clearTimeout(timer); delete host[callback]; error ? reject(error) : resolve(output); };
    const timer = setTimeout(() => finish(new Error('管理器桥接超时，请检查 WebUI 支持与服务状态')), 12000);
    host[callback] = (code: number, stdout: string, stderr: string) => finish(code === 0 ? null : new Error(stderr?.trim() || '本机管理命令失败'), stdout);
    try {
      let output: unknown;
      try { output = host.ksu!.exec(text, '{}', callback); }
      catch { output = host.ksu!.exec(text); }
      if (typeof output === 'string') finish(null, output);
    } catch (error) { finish(error as Error); }
  });
}
export async function request<T = Record<string, unknown>>(payload: unknown): Promise<T> {
  const output = await command(`${control} request ${encodeRequest(payload)}`);
  const value = JSON.parse(output.trim());
  if (value.ok === false) throw new Error(value.error || '操作失败');
  return value;
}
export async function service(action: 'start' | 'stop' | 'install'): Promise<void> {
  await command(`${control} ${action}`);
}
