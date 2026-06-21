package format

// Reader Validation-Mode (RVM) lets a format reader emit non-fatal, located
// structure/encoding diagnostics while it keeps extracting leniently. It is
// opt-in and DEFAULT-OFF: the zero ValidationMode (ValidationOff) leaves a
// reader's behavior byte-identical to before — no diagnostics are recorded and
// the lenient extraction path is untouched. The `kapi check --validate` flag
// turns it on and folds the diagnostics into the kapi.check/v1 Report.
//
// The contract lives in core/format and imports nothing from core/check: the
// framework's format layer must stay platform-agnostic. The check layer maps
// these into its own Diagnostic (see core/check.DiagnosticFromReader).

// ValidationMode controls whether a reader records validation diagnostics.
// The zero value is ValidationOff so a reader that never sees an explicit mode
// behaves exactly as it did before RVM.
type ValidationMode int

const (
	// ValidationOff is the default: no diagnostics, byte-identical lenient
	// extraction.
	ValidationOff ValidationMode = iota
	// ValidationReport records non-fatal diagnostics alongside the lenient
	// extraction; the caller surfaces them but does not gate on them.
	ValidationReport
	// ValidationStrict records the same diagnostics; the check layer treats a
	// structure/encoding problem of Major severity or worse as a gate failure.
	ValidationStrict
)

// Severity ranks a Diagnostic's impact. It is a local type so core/format
// imports nothing from the platform-side core/check; check maps these onto its
// own Severity (DiagnosticFromReader). The four levels mirror check's MQM-style
// scale (critical/major/minor/neutral).
type Severity string

const (
	// SeverityCritical is a release-blocking integrity problem.
	SeverityCritical Severity = "critical"
	// SeverityMajor is a clear structural/encoding violation.
	SeverityMajor Severity = "major"
	// SeverityMinor is a low-impact problem (e.g. a relabeled charset, an
	// unavailable subfilter).
	SeverityMinor Severity = "minor"
	// SeverityNeutral is informational; it carries no penalty.
	SeverityNeutral Severity = "neutral"
)

// Diagnostic is one located, non-fatal structure or encoding problem a reader
// surfaced while extracting leniently. Category is a dotted id from the
// structure.* / encoding.* families (e.g. "structure.json-syntax",
// "encoding.invalid-utf8"); the check layer derives its rule and check family
// from it. Position fields are 1-based line/column plus the byte offset; a
// reader that cannot localize a problem (the shallow HTML leg) leaves them zero
// and reports document-level.
type Diagnostic struct {
	Severity   Severity
	Category   string
	Message    string
	Line       int
	Column     int
	ByteOffset int
	Snippet    string
}

// DiagnosticReader is implemented by readers that record validation
// diagnostics. Discovery is by assertion: the DataFormatReader interface is
// unchanged, so every existing reader keeps compiling and a caller opts in with
// a type assert (`if dr, ok := reader.(format.DiagnosticReader); ok { … }`).
// BaseFormatReader satisfies this for free, so any reader that embeds it is a
// DiagnosticReader without extra code.
type DiagnosticReader interface {
	// Diagnostics returns the diagnostics recorded during Read. It is queried
	// after the Read range loop completes (the state lives on the reader, not
	// the channel) and before Close.
	Diagnostics() []Diagnostic
}

// ValidationConfig is the optional interface a DataFormatConfig implements to
// carry the reader validation mode. A config gets it for free by embedding
// ValidationConfigField. A caller (the CLI) sets the mode before Open via
// SetValidationMode; the reader reads it back through
// BaseFormatReader.ValidationMode.
type ValidationConfig interface {
	ValidationMode() ValidationMode
	SetValidationMode(ValidationMode)
}

// ValidationConfigField is embedded by format configs to carry the reader
// validation mode. The zero value is ValidationOff, so a config that embeds it
// defaults to the byte-identical lenient path regardless of how it is
// constructed. The field is excluded from serialization: the mode is a
// per-invocation setting, never persisted in a project/config envelope.
type ValidationConfigField struct {
	Validation ValidationMode `json:"-" yaml:"-"`
}

// ValidationMode reports the configured reader validation mode.
func (v *ValidationConfigField) ValidationMode() ValidationMode { return v.Validation }

// SetValidationMode sets the reader validation mode.
func (v *ValidationConfigField) SetValidationMode(m ValidationMode) { v.Validation = m }

// LineColumn computes the 1-based line and column for a byte offset into src.
// Offsets past the end clamp to the end of src; a negative offset returns
// (1, 1). Shared by the readers and the encoding helper so every located
// Diagnostic reports positions the same way.
func LineColumn(src []byte, offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line, col = 1, 1
	for i := range offset {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// SnippetAround returns a short, single-line excerpt of src around offset for a
// Diagnostic.Snippet: the line containing offset, trimmed to at most max bytes.
// It never spans a newline, so the snippet stays printable in a one-line
// finding. A max of 0 uses a default of 80.
func SnippetAround(src []byte, offset, max int) string {
	if max <= 0 {
		max = 80
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	start := offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(src) && src[end] != '\n' {
		end++
	}
	line := src[start:end]
	if len(line) > max {
		line = line[:max]
	}
	return string(line)
}
