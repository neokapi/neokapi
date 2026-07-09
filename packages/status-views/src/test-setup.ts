import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// Component tests render into jsdom; unmount between cases to avoid DOM bleed.
afterEach(cleanup);
