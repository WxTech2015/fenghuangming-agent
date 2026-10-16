import { expect, test } from 'vitest';
import { encodeRequest } from './bridge';
test('管理请求中的中文与 Shell 符号仅以 Base64 数据传递', () => {
  const input = { action: 'configure', value: "晚霞';$(touch /tmp/unsafe)\n" };
  const encoded = encodeRequest(input);
  expect(encoded).toMatch(/^[A-Za-z0-9+/=]+$/);
  expect(JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(encoded), c => c.charCodeAt(0))))).toEqual(input);
});
test('拒绝超长本机请求', () => { expect(() => encodeRequest({ token: 'x'.repeat(17000) })).toThrow('过大'); });
