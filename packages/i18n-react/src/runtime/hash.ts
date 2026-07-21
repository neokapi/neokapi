/**
 * Browser-safe key hashing — a byte-for-byte port of `plugin/hash.ts`.
 *
 * The build-side hasher (`plugin/hash.ts`) reads UTF-8 bytes via Node's
 * `Buffer`, which isn't available in the browser runtime. The document-head
 * hooks (`@neokapi/i18n-react/head`) must recompute a block's hash at render
 * time — the transform never rewrites them (a head string has no DOM text
 * node) — so they need this port. `TextEncoder` yields the same UTF-8 bytes
 * `Buffer.from(str, "utf8")` does, so the FNV-1a/base62 result is identical to
 * the extractor's `Block.hash`. The `hash-parity` test guards that equivalence.
 */

const FNV_OFFSET = 0xcbf29ce484222325n;
const FNV_PRIME = 0x100000001b3n;
const MASK64 = 0xffffffffffffffffn;

const encoder = new TextEncoder();

function fnv1a64(str: string): bigint {
  // Hash UTF-8 bytes so non-ASCII text hashes identically to the Node-side
  // `Buffer`-based hasher (and any future Go implementation).
  const bytes = encoder.encode(str);
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
 * Compute the hash key from flat text and descriptor. Identical output to
 * `plugin/hash.ts#hashKey` for any input.
 */
export function hashKey(text: string, desc: string): string {
  const key = JSON.stringify(text) + "|" + desc;
  return bigintToBase62(fnv1a64(key));
}
