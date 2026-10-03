// Package changes is the Bowrain server's side of the change contract
// (kapi.change/v1, core/change): the stream home, which keeps a stream's
// documents in rows, the operations a server job sends for the blocks a tool
// produced, and the recorder that announces an applied change set.
//
// A stream's document is an item, addressed by its path. Each of its blocks is
// a row holding every edition of the block: the source and each translation.
// The home reads an item's blocks by key, applies a change set's operations to
// them in memory, and commits by writing the rows in one transaction that
// holds them from the read that decided the change, so an operation's
// if_match is checked against the rows the write stores. block_history and
// change_log are written on the same transaction.
//
// The server's policy, commit check and decisions sit in package server,
// where the request's permissions and the governance resolution live; the
// server MCP builds its service here with an agent's policy.
package changes
