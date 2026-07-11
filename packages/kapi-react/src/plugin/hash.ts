/**
 * v2 key hashing: FNV-1a 64-bit over `JSON.stringify(text) + "|" + desc`,
 * base62-encoded (≤ 11 chars).
 *
 * 64 bits gives collision headroom for million-string corpora that the
 * old 32-bit Jenkins hash (~50% birthday collision odds around 80k
 * strings) did not. The v1 scheme lives on only in
 * `src/migrate/legacy.ts` for `kapi-react migrate-keys`.
 */

const FNV_OFFSET = 0xcbf29ce484222325n;
const FNV_PRIME = 0x100000001b3n;
const MASK64 = 0xffffffffffffffffn;

function fnv1a64(str: string): bigint {
  // Hash UTF-8 bytes so non-ASCII text hashes identically across
  // JS engines and the Go implementation, should one ever exist.
  const bytes = Buffer.from(str, "utf8");
  let hash = FNV_OFFSET;
  for (let i = 0; i < bytes.length; i++) {
    hash ^= BigInt(bytes[i]);
    hash = (hash * FNV_PRIME) & MASK64;
  }
  return hash;
}

const BaseNSymbols = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";

function bigintToBase62(value: bigint): string {
  if (value === 0n) return "0";
  let n = value;
  let out = "";
  while (n > 0n) {
    out = BaseNSymbols[Number(n % 62n)] + out;
    n /= 62n;
  }
  return out;
}

/**
 * Compute the hash key from flat text and descriptor.
 */
export function hashKey(text: string, desc: string): string {
  const key = JSON.stringify(text) + "|" + desc;
  return bigintToBase62(fnv1a64(key));
}
