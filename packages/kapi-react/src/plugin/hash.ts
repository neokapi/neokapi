/**
 * Jenkins hash + base62 encoding for hash keys.
 * Copied from babel-plugin-neokapi to avoid a runtime dependency on it.
 */

function toUtf8(str: string) {
  const result = [];
  const len = str.length;
  for (let i = 0; i < len; i++) {
    let charcode = str.charCodeAt(i);
    if (charcode < 0x80) {
      result.push(charcode);
    } else if (charcode < 0x8_00) {
      result.push(0xc0 | (charcode >> 6), 0x80 | (charcode & 0x3f));
    } else if (charcode < 0xd8_00 || charcode >= 0xe0_00) {
      result.push(
        0xe0 | (charcode >> 12),
        0x80 | ((charcode >> 6) & 0x3f),
        0x80 | (charcode & 0x3f),
      );
    } else {
      i++;
      charcode = 0x1_00_00 + (((charcode & 0x3_ff) << 10) | (str.charCodeAt(i) & 0x3_ff));
      result.push(
        0xf0 | (charcode >> 18),
        0x80 | ((charcode >> 12) & 0x3f),
        0x80 | ((charcode >> 6) & 0x3f),
        0x80 | (charcode & 0x3f),
      );
    }
  }
  return result;
}

function jenkinsHash(str: string): number {
  if (!str) return 0;
  const utf8 = toUtf8(str);
  let hash = 0;
  for (let i = 0; i < utf8.length; i++) {
    hash += utf8[i];
    hash = (hash + (hash << 10)) >>> 0;
    hash ^= hash >>> 6;
  }
  hash = (hash + (hash << 3)) >>> 0;
  hash ^= hash >>> 11;
  hash = (hash + (hash << 15)) >>> 0;
  return hash;
}

const BaseNSymbols = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";

function uintToBaseN(numberArg: number, base: number) {
  let number = numberArg;
  if (base < 2 || base > 62 || number < 0) return "";
  let output = "";
  do {
    output = BaseNSymbols.charAt(number % base).concat(output);
    number = Math.floor(number / base);
  } while (number > 0);
  return output;
}

/**
 * Compute the hash key from text and description.
 * Compute the hash key from text and description.
 */
export function hashKey(text: string, desc: string): string {
  const key = JSON.stringify(text) + "|" + desc;
  return uintToBaseN(jenkinsHash(key), 62);
}
