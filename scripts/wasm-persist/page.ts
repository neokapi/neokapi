// The page the persistence test drives: it boots the engine through the
// @neokapi/engine facade, as the playground does, and leaves the runtime on
// window for the test to call.
import { bootKapiRuntime } from "../../packages/engine/src/runtime.ts";
import type { BootOptions, KapiRuntime, StorageInfo } from "../../packages/engine/src/runtime.ts";

declare global {
  interface Window {
    boot(opts?: BootOptions): Promise<KapiRuntime>;
    rt?: KapiRuntime;
    /** Every storage change the runtime reported. */
    storageChanges: StorageInfo[];
  }
}

window.storageChanges = [];
window.boot = async (opts?: BootOptions) => {
  window.rt = await bootKapiRuntime("/wasm/wasm_exec.js", "/wasm/kapi-cli.wasm", opts);
  window.rt.onStorageChange((info) => window.storageChanges.push(info));
  return window.rt;
};
