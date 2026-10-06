// The page the persistence test drives: it boots the engine through the
// @neokapi/engine facade, as the playground does, and leaves the runtime on
// window for the test to call.
import { bootKapiRuntime } from "../../packages/engine/src/runtime.ts";
import type { KapiRuntime } from "../../packages/engine/src/runtime.ts";

declare global {
  interface Window {
    boot(): Promise<KapiRuntime>;
    rt?: KapiRuntime;
  }
}

window.boot = async () => {
  window.rt = await bootKapiRuntime("/wasm/wasm_exec.js", "/wasm/kapi-cli.wasm");
  return window.rt;
};
