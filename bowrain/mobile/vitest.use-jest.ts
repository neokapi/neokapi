// Vitest's global setup for bowrain/mobile: the tests here run on jest-expo,
// so `vp test` stops at once with one line that names the command that runs
// them.
export default function setup(): never {
  process.stderr.write(
    "bowrain/mobile runs its tests on jest-expo: use `vp run test` (or `vpr test`).\n",
  );
  process.exit(1);
}
