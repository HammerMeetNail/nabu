// qr.js — QR codes for invite links (ISO/IEC 18004): byte mode, error
// correction level M, versions 1–10, drawn as SVG. Only what an invite link
// needs: an app URL plus a 10-character code fits in version 3 or 4.

import { escapeHTML } from "./utils.js";

// Level M blocks per version: [EC codewords per block, data codewords of each
// block]. Shorter blocks come first, as the standard interleaves them.
const BLOCKS = [
  null,
  [10, [16]],
  [16, [28]],
  [26, [44]],
  [18, [32, 32]],
  [24, [43, 43]],
  [16, [27, 27, 27, 27]],
  [18, [31, 31, 31, 31]],
  [22, [38, 38, 39, 39]],
  [22, [36, 36, 36, 37, 37]],
  [26, [43, 43, 43, 43, 44]],
];

// Alignment pattern centres per version.
const ALIGNMENT = [null, [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50]];

export const QR_MAX_BYTES = 213;

// GF(256) with the QR polynomial x^8 + x^4 + x^3 + x^2 + 1.
const EXP = new Uint8Array(510);
const LOG = new Uint8Array(256);
for (let i = 0, x = 1; i < 255; i++) {
  EXP[i] = EXP[i + 255] = x;
  LOG[x] = i;
  x <<= 1;
  if (x & 0x100) x ^= 0x11d;
}
const gfMul = (a, b) => (a && b ? EXP[LOG[a] + LOG[b]] : 0);

// Reed–Solomon error correction codewords for one block.
export function reedSolomon(data, ecLength) {
  // Generator (x - a^0)(x - a^1)…(x - a^(n-1)), highest power first.
  let generator = [1];
  for (let i = 0; i < ecLength; i++) {
    const next = new Array(generator.length + 1).fill(0);
    generator.forEach((c, j) => {
      next[j] ^= c;
      next[j + 1] ^= gfMul(c, EXP[i]);
    });
    generator = next;
  }
  const remainder = new Array(ecLength).fill(0);
  for (const byte of data) {
    const factor = byte ^ remainder.shift();
    remainder.push(0);
    for (let j = 0; j < ecLength; j++) remainder[j] ^= gfMul(generator[j + 1], factor);
  }
  return remainder;
}

const dataCapacity = version => BLOCKS[version][1].reduce((sum, n) => sum + n, 0);
const countBits = version => (version < 10 ? 8 : 16);

function chooseVersion(byteLength) {
  for (let version = 1; version < BLOCKS.length; version++) {
    if (4 + countBits(version) + byteLength * 8 <= dataCapacity(version) * 8) return version;
  }
  throw new RangeError(`A QR code holds at most ${QR_MAX_BYTES} bytes`);
}

// The data codewords: byte mode, the length, the bytes, a terminator and pad.
function dataCodewords(bytes, version) {
  const capacity = dataCapacity(version);
  const bits = [];
  const push = (value, length) => {
    for (let i = length - 1; i >= 0; i--) bits.push((value >>> i) & 1);
  };
  push(0b0100, 4);
  push(bytes.length, countBits(version));
  bytes.forEach(byte => push(byte, 8));
  push(0, Math.min(4, capacity * 8 - bits.length));
  push(0, (8 - (bits.length % 8)) % 8);
  const codewords = [];
  for (let i = 0; i < bits.length; i += 8) {
    codewords.push(bits.slice(i, i + 8).reduce((byte, bit) => (byte << 1) | bit, 0));
  }
  for (let pad = 0xec; codewords.length < capacity; pad ^= 0xec ^ 0x11) codewords.push(pad);
  return codewords;
}

// Data and error correction codewords, interleaved across blocks.
function codewords(bytes, version) {
  const [ecLength, sizes] = BLOCKS[version];
  const data = dataCodewords(bytes, version);
  const blocks = [];
  let offset = 0;
  for (const size of sizes) {
    const block = data.slice(offset, offset + size);
    offset += size;
    blocks.push({ data: block, ec: reedSolomon(block, ecLength) });
  }
  const out = [];
  for (let i = 0; i < Math.max(...sizes); i++) {
    blocks.forEach(block => { if (i < block.data.length) out.push(block.data[i]); });
  }
  for (let i = 0; i < ecLength; i++) blocks.forEach(block => out.push(block.ec[i]));
  return out;
}

// 15 format bits: level M (00) and the mask, BCH-protected and XOR-masked.
export function formatBits(mask) {
  let remainder = mask;
  for (let i = 0; i < 10; i++) remainder = (remainder << 1) ^ ((remainder >>> 9) * 0x537);
  return ((mask << 10) | remainder) ^ 0x5412;
}

// 18 version bits (versions 7 and up), BCH-protected.
export function versionBits(version) {
  let remainder = version;
  for (let i = 0; i < 12; i++) remainder = (remainder << 1) ^ ((remainder >>> 11) * 0x1f25);
  return (version << 12) | remainder;
}

const MASKS = [
  (x, y) => (x + y) % 2 === 0,
  (x, y) => y % 2 === 0,
  (x, y) => x % 3 === 0,
  (x, y) => (x + y) % 3 === 0,
  (x, y) => (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0,
  (x, y) => ((x * y) % 2) + ((x * y) % 3) === 0,
  (x, y) => (((x * y) % 2) + ((x * y) % 3)) % 2 === 0,
  (x, y) => (((x + y) % 2) + ((x * y) % 3)) % 2 === 0,
];

// The standard's penalty score; the mask with the lowest one is used.
function penalty(modules) {
  const size = modules.length;
  let score = 0;
  for (let i = 0; i < size; i++) {
    for (const line of [modules[i], modules.map(row => row[i])]) {
      // Runs of five or more modules of one color.
      let run = 1;
      for (let j = 1; j <= size; j++) {
        if (j < size && line[j] === line[j - 1]) {
          run++;
        } else {
          if (run >= 5) score += run - 2;
          run = 1;
        }
      }
      // Finder-like 1:1:3:1:1 patterns with four light modules on one side
      // (beyond the edge counts as light).
      const light = j => j < 0 || j >= size || !line[j];
      for (let j = 0; j + 7 <= size; j++) {
        if (line[j] && !line[j + 1] && line[j + 2] && line[j + 3] && line[j + 4] && !line[j + 5] && line[j + 6] &&
            ([1, 2, 3, 4].every(k => light(j - k)) || [7, 8, 9, 10].every(k => light(j + k)))) {
          score += 40;
        }
      }
    }
  }
  // 2×2 blocks of one color.
  for (let y = 0; y + 1 < size; y++) {
    for (let x = 0; x + 1 < size; x++) {
      const c = modules[y][x];
      if (c === modules[y][x + 1] && c === modules[y + 1][x] && c === modules[y + 1][x + 1]) score += 3;
    }
  }
  // Distance of the dark share from half, in 5% steps.
  const dark = modules.reduce((n, row) => n + row.filter(Boolean).length, 0);
  score += Math.floor(Math.abs(dark * 20 - size * size * 10) / (size * size)) * 10;
  return score;
}

// The module matrix for text: modules[y][x] is true for a dark module.
// `mask` forces a mask pattern (tests only).
export function encodeQR(text, { mask } = {}) {
  const bytes = Array.from(new TextEncoder().encode(String(text)));
  const version = chooseVersion(bytes.length);
  const size = version * 4 + 17;
  const modules = Array.from({ length: size }, () => new Array(size).fill(false));
  const reserved = Array.from({ length: size }, () => new Array(size).fill(false));
  const set = (x, y, dark) => {
    modules[y][x] = dark;
    reserved[y][x] = true;
  };

  for (let i = 0; i < size; i++) {
    set(6, i, i % 2 === 0);
    set(i, 6, i % 2 === 0);
  }
  for (const [cx, cy] of [[3, 3], [size - 4, 3], [3, size - 4]]) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const x = cx + dx, y = cy + dy;
        const ring = Math.max(Math.abs(dx), Math.abs(dy));
        if (x >= 0 && y >= 0 && x < size && y < size) set(x, y, ring !== 2 && ring !== 4);
      }
    }
  }
  const centres = ALIGNMENT[version];
  const last = size - 7;
  for (const cy of centres) {
    for (const cx of centres) {
      if ((cx === 6 && cy === 6) || (cx === 6 && cy === last) || (cx === last && cy === 6)) continue;
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) set(cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
      }
    }
  }
  const drawFormat = m => {
    const bits = formatBits(m);
    const bit = i => ((bits >>> i) & 1) === 1;
    for (let i = 0; i <= 5; i++) set(8, i, bit(i));
    set(8, 7, bit(6));
    set(8, 8, bit(7));
    set(7, 8, bit(8));
    for (let i = 9; i < 15; i++) set(14 - i, 8, bit(i));
    for (let i = 0; i < 8; i++) set(size - 1 - i, 8, bit(i));
    for (let i = 8; i < 15; i++) set(8, size - 15 + i, bit(i));
    set(8, size - 8, true);
  };
  drawFormat(0);
  if (version >= 7) {
    const bits = versionBits(version);
    for (let i = 0; i < 18; i++) {
      const dark = ((bits >>> i) & 1) === 1;
      const a = size - 11 + (i % 3), b = Math.floor(i / 3);
      set(a, b, dark);
      set(b, a, dark);
    }
  }

  // Codewords zigzag up and down two-module columns from the bottom right,
  // skipping the vertical timing pattern; leftover remainder bits stay light.
  const data = codewords(bytes, version);
  let bit = 0;
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5;
    const upward = ((right + 1) & 2) === 0;
    for (let step = 0; step < size; step++) {
      const y = upward ? size - 1 - step : step;
      for (const x of [right, right - 1]) {
        if (reserved[y][x] || bit >= data.length * 8) continue;
        modules[y][x] = ((data[bit >>> 3] >>> (7 - (bit & 7))) & 1) === 1;
        bit++;
      }
    }
  }

  const applyMask = m => {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        if (!reserved[y][x] && MASKS[m](x, y)) modules[y][x] = !modules[y][x];
      }
    }
  };
  let chosen = mask;
  if (chosen === undefined) {
    let lowest = Infinity;
    for (let m = 0; m < MASKS.length; m++) {
      applyMask(m);
      drawFormat(m);
      const score = penalty(modules);
      if (score < lowest) {
        lowest = score;
        chosen = m;
      }
      applyMask(m);
    }
  }
  applyMask(chosen);
  drawFormat(chosen);
  return { version, mask: chosen, size, modules };
}

// An SVG QR code for text, dark on white whatever the theme, with the
// standard four-module quiet zone. `label` names it for assistive technology.
export function renderQRCode(text, label) {
  const { size, modules } = encodeQR(text);
  const quiet = 4;
  const extent = size + quiet * 2;
  let path = "";
  modules.forEach((row, y) => {
    for (let x = 0; x < size; x++) {
      if (!row[x]) continue;
      let run = 1;
      while (x + run < size && row[x + run]) run++;
      path += `M${x + quiet} ${y + quiet}h${run}v1h-${run}z`;
      x += run - 1;
    }
  });
  return `<svg class="qr-code" viewBox="0 0 ${extent} ${extent}" role="img" aria-label="${escapeHTML(label)}" shape-rendering="crispEdges"><rect width="${extent}" height="${extent}" fill="#fff"/><path d="${path}" fill="#000"/></svg>`;
}
