// @neokapi/kapi-learn: the learning labs. A curriculum of scripted sessions in
// the sample projects, a lab shell that runs them against the browser engine
// (and in Node, for the verifier), and the player that presents a lab the way
// a video site presents a video: chapters, a transport bar, deep links.
//
// Framework-agnostic: no @docusaurus/* imports. The host supplies the engine
// asset URLs. The light modules (curriculum, shell, deep links, progress) are
// SSR-clean; the player pulls in xterm and the engine boot path and is meant
// to be loaded as a separate chunk.

export * from "./curriculum/index.ts";
export { SAMPLE_TREES } from "./samples.gen.ts";
export type { SampleFile, SampleTreeId } from "./samples.gen.ts";
export * from "./shell/index.ts";
export * from "./deeplink.ts";
export * from "./progress.ts";
