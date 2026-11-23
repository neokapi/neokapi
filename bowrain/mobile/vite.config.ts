// The app's tests run on jest-expo (`vp run test`, which is `vpr test`), the
// preset that provides React Native's test environment. `vp test` starts
// Vitest, which finds the same files and runs none of them, so it stops here
// with the command to use.
import { defineConfig } from "vite-plus";

export default defineConfig({
  test: {
    globalSetup: ["./vitest.use-jest.ts"],
  },
});
