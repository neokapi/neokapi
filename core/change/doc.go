// Package change is the one way content changes in neokapi: the kapi.change/v1
// contract and the code that applies it to a block.
//
// A change set (Set) is an envelope of ordered operations (Op). Each content
// operation addresses one edition of one block with a Ref, {doc, block,
// edition}, and names the revision of that edition its sender read
// (IfMatch). The revision is a function of the edition's content alone
// (model.EditionRevision), so the same token holds wherever the content is
// kept. A sender that read an older revision is refused with the current
// content, and nothing is written.
//
// The package holds:
//
//   - the contract types: the envelope, the operation union, references,
//     results, and the closed set of error codes with their exit codes and
//     HTTP statuses (errors.go);
//   - strict decoding: an unknown field or operation is refused as invalid
//     with the JSON pointer of what was wrong (Decode);
//   - a description of each operation (Operations), from which package
//     changeschema generates the JSON Schema of a change set;
//   - ApplyBlock, which applies content operations to one block in memory
//     under the rules every content operation obeys: text is a form of runs
//     parsed against a reference code set, inline codes keep their editing
//     constraints, plural and select structure is kept, run flags survive,
//     and overlays on the edited edition follow the edit;
//   - Capabilities, what a format's writer declares it can write beyond what
//     its reader read (the attributes set_attribute may change, the types
//     mark may create), with the writer that spells them, and FormatOps,
//     which builds the table a format publishes from those declarations;
//   - Consequences, which says what an applied edit does to an edition's
//     status and origin;
//   - Diff, which turns a pair of blocks into the operations that make one
//     into the other, so a path that arrives with a whole new block goes
//     through the same rules as a typed operation;
//   - the Service, which reads documents in their homes, applies a change set
//     to every document it names or to none, and describes what a format
//     supports (DescribeFormat), with the hooks a host plugs in: the commit
//     check, the policy, the recorder, the asset operations and the editions'
//     states.
//
// The package imports core/model and core/format and nothing above them: no
// host, no CLI, no surface. Every surface (the CLI, MCP, Kapi Desktop, the browser build, tools
// in flows and the Bowrain server) can build on it, and core/tool does, so a
// tool's view writes through ApplyBlock.
package change
