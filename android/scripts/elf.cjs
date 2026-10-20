function androidApi(binary) {
  if (!binary.subarray(0, 6).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46, 2, 1]))) throw new Error('需要 ELF64 小端文件');
  const table = Number(binary.readBigUInt64LE(40)), size = binary.readUInt16LE(58), count = binary.readUInt16LE(60);
  const names = table + size * binary.readUInt16LE(62);
  if (size < 64 || table + size * count > binary.length || names + 64 > binary.length) throw new Error('ELF 节表无效');
  const strings = Number(binary.readBigUInt64LE(names + 24));
  for (let index = 0; index < count; index++) {
    const section = table + size * index, offset = strings + binary.readUInt32LE(section);
    if (binary.subarray(offset, binary.indexOf(0, offset)).toString() !== '.note.android.ident') continue;
    const note = Number(binary.readBigUInt64LE(section + 24)), length = Number(binary.readBigUInt64LE(section + 32));
    if (note + length > binary.length || length < 24) throw new Error('Android ELF 标记损坏');
    const nameSize = binary.readUInt32LE(note), description = note + 12 + Math.ceil(nameSize / 4) * 4;
    if (binary.subarray(note + 12, note + 12 + nameSize).toString().replace(/\0+$/, '') !== 'Android' || description + 4 > note + length) throw new Error('Android ELF 标记无效');
    return binary.readUInt32LE(description);
  }
  throw new Error('缺少 Android API 编译标记');
}
module.exports = { androidApi };
