// Package format defines the interfaces for reading and writing localization
// file formats. A [DataFormatReader] parses a document into a stream of Parts,
// and a [DataFormatWriter] reconstructs a document from that stream. Concrete
// format implementations live in core/formats/.
//
// Both interfaces are compositions of narrower facets: [FormatDescriptor]
// (identity + detection metadata), [Configurable] (typed config), and
// [PartReader] for readers; [OutputSink] and [PartWriter] for writers.
// Consumers that need only one facet can depend on it directly.
//
// # Optional capability interfaces
//
// Beyond the core interfaces, a format opts into extra behavior by
// implementing an optional capability interface. Discovery is by type
// assertion at the call site (the file-run path, the CLI, the registry); a
// format that doesn't implement a capability simply keeps the default path.
// The capabilities are:
//
// Reader-side:
//
//   - [StreamingReader] — bare marker: the reader consumes its input
//     incrementally (bounded memory) and is in-process, so the file-run path
//     may stream it straight into the executor. Implemented by line/token
//     oriented text formats (json, properties, srt, i18next). Probe with
//     [IsStreamingReader].
//   - [SkeletonStoreEmitter] — the reader records the document's
//     non-translatable scaffolding into a [SkeletonStore] so a same-format
//     writer can reproduce the document byte-for-byte around the edited text.
//     Implemented by round-trip formats.
//   - [SubfilterAware] — the reader (or writer) delegates embedded content
//     (HTML inside JSON, …) to another format via a [SubfilterResolver] the
//     pipeline injects before Open.
//   - [DiagnosticReader] — the reader records Reader-Validation-Mode
//     diagnostics during Read. BaseFormatReader implements it, so every
//     embedding reader has it for free.
//   - [PreviewBuilder] — the reader generates a visual HTML preview with
//     <kat-block> markers for the editor; others get a generic fallback.
//
// Writer-side:
//
//   - [StreamingWriter] — bare marker: the writer can consume a *streaming*
//     skeleton interleaved with the Part stream for a bounded-memory
//     round-trip. Wired only when the reader is a StreamingReader too. Probe
//     with [IsStreamingWriter].
//   - [SkeletonStoreConsumer] — the writer injects translated text back into
//     a skeleton recorded at read time (byte-exact same-format fidelity).
//     Orthogonal to being generative: a writer may use a skeleton when
//     offered and still write standalone without one.
//   - [OriginalContentSetter] — the writer needs the original document bytes
//     as its skeleton (bridge writers, native HTML).
//   - [SourcePathSetter] — the writer can read the original document from a
//     path instead of receiving content bytes (avoids buffering/gRPC
//     transfer for shared filesystems).
//   - [SourceLocaleSetter] — the writer needs the source locale in addition
//     to the target locale (e.g. OOXML rewrites locale-bearing attributes
//     whose value matches the source).
//   - [WriterConfigurable] — the writer exposes serialization knobs through
//     the same DataFormatConfig contract readers use.
//   - [OutputConfigurable] — the writer accepts the shared byte-level output
//     options (BOM, newline style, charset). BaseFormatWriter implements it.
//   - [GenerativeWriter] — declares whether the writer can serialize a
//     complete document from the content model alone (no skeleton), making
//     the format a valid cross-format conversion target. BaseFormatWriter
//     implements it via the RequiresSkeleton declaration.
//   - [InterchangeWriter] — declares a bilingual translation-interchange
//     format (XLIFF, PO, TMX, …), excluded from `convert` targets.
//     BaseFormatWriter implements it via the Interchange field.
package format
