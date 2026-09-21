import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { kapiIsolationEnv } from "./paths.ts";

test("every kapi root the isolation contract names is set", () => {
  const env = kapiIsolationEnv();
  for (const name of [
    "XDG_DATA_HOME",
    "XDG_CACHE_HOME",
    "KAPI_DATA_DIR",
    "KAPI_CONFIG_DIR",
    "KAPI_PLUGINS_DIR",
  ]) {
    assert.equal(path.isAbsolute(env[name]), true, `${name} names an absolute throwaway directory`);
  }
  assert.equal(env.KAPI_PLUGINS_DIR_ONLY, "1");
  assert.equal(env.KAPI_NO_PROJECT, "1");
  assert.equal(env.KAPI_TELEMETRY, "0");
});

// A recording made from a coding agent's shell inherits that host's marker
// variable, and kapi reads a marker as the agent driving the command. The
// operations a demo records belong to the person the walkthrough shows.
test("a recording is attributed to a person whatever shell it runs in", () => {
  assert.equal(kapiIsolationEnv().KAPI_ACTOR, "person");
});
