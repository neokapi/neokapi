# Path guard redesign: state of the work

The redesign covers three things. Allowlists move inline, reasons stay checked, and the scope widens to every module.

- **Inline allowlists.** The per-file allowlist maps in `scripts/pathguard/allow_*.go` are replaced by inline `//pathguard: <reason>` comments at each by-path call. The comment sits on the call's line, or alone on the line above.
- **Checked reasons.** A comment with no reason is reported, and so is one on a line that reaches no file.
- **Wider scope.** The guard type-checks every package of every module in `go.work`, not only the packages that handle project content.
  - Besides calls, it reports function values and `net/http.Dir` conversions.
  - It treats as a wrapper any function whose allowed call takes one of its own parameters as the path, and reports each wrapper's callers.
  - `-config` picks a build configuration.

Done, all uncommitted until this commit:
- `main.go` and `analyze.go` hold the new analysis, and `selftest.go` has fixtures for comments, wrappers, function values and `http.Dir`.
- The `allow_*.go` files are deleted, and about 580 `//pathguard:` comments are placed across the tree.
- `host/pathroots.go` declares the directories a plugin daemon may be sent paths in (`pluginhost.AddPathRoots`).

Left to do:
- The branch is far behind main. It was cut before the state move (#3780, #3804, #3806, #3813), which rewrote or removed many allowlist reasons and added or changed by-path calls. Merge main, convert every entry those PRs touched into a comment, and drop comments on calls that are gone.
- Run `go run ./scripts/pathguard -self-test` and `go run ./scripts/pathguard` under each configuration until clean. Nothing here has been built or tested since the last commit.
- Open the PR with the guard change stated at the top, since it changes a check. Get the founder's sign-off, because widening the guard and moving its allowlist inline both change the check.
