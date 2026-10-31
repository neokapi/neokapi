// The fixtures are the JSON host/screens marshals for each screen
// (TestFixtures in host/screens/fixtures_test.go writes and pins them), typed
// as the views read them. The `satisfies` checks run in the package typecheck,
// so a field the Go side drops or renames fails it.

import mapJSON from "../fixtures/map-summary.json";
import readerJSON from "../fixtures/reader-file.json";
import decideJSON from "../fixtures/rules-decide-view.json";
import type { MapSummary, ReaderFile, RulesDecideView } from "./types";

// A JSON import types each string as `string`, never as one of a union's
// literals, so the fixtures satisfy the types with those literals widened.
// The tests in __tests__/fixtures.test.ts assert the literals themselves.
type Loose<T> = T extends string
  ? string
  : T extends number | boolean | null | undefined
    ? T
    : T extends readonly (infer U)[]
      ? Loose<U>[]
      : { [K in keyof T]: Loose<T[K]> };

export const mapSummaryFixture = mapJSON satisfies Loose<MapSummary> as MapSummary;
export const readerFileFixture = readerJSON satisfies Loose<ReaderFile> as ReaderFile;
export const rulesDecideViewFixture =
  decideJSON satisfies Loose<RulesDecideView> as RulesDecideView;
