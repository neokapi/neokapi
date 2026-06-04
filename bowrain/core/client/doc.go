// Package client implements the Bowrain Merkle-diff push protocol and the
// accompanying REST helpers for workspace, project, stream, asset, and token
// management. The primary type is BowrainClient, which drives the full
// init → diff → chunk-upload → commit push cycle over the sync API.
package client
