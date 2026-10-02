package change

import (
	"fmt"
	"slices"
)

// Code is why an operation or a change set was refused. The set is closed:
// every transport maps each code once (ExitCode, HTTPStatus), and each code
// tells the sender which of a few things to do next.
type Code string

const (
	// CodeInvalid: the change set does not decode, fails its schema, or
	// contradicts itself. The error carries the JSON pointer. Fix the change set.
	CodeInvalid Code = "invalid"
	// CodeNotFound: a reference resolves to no document, block or edition, or a
	// find matches nothing. Retarget or re-read.
	CodeNotFound Code = "not_found"
	// CodeAmbiguous: a key or a find matches more than one thing. The error
	// carries the candidates. Narrow it.
	CodeAmbiguous Code = "ambiguous"
	// CodeStale: if_match no longer holds, or require_basis is set and the
	// basis moved; Field says which. The result carries the current revision
	// and content. Re-read, rebase and resend.
	CodeStale Code = "stale"
	// CodeDocChanged: the document changed under the commit and one retry
	// could not settle it. Resend.
	CodeDocChanged Code = "doc_changed"
	// CodeGuard: the result would drop, add or unbalance inline codes, or
	// flatten structure (Subcode says which). Fix the content.
	CodeGuard Code = "guard"
	// CodeGateFailed: a failing rule finds a violation the edit introduces.
	// Fix the wording, or a person overrides.
	CodeGateFailed Code = "gate_failed"
	// CodeUnsupported: the format, edition or home lacks the capability the
	// operation needs. The error names it.
	CodeUnsupported Code = "unsupported"
	// CodeNotPermitted: the sender's policy refuses the operation. Ask a person.
	CodeNotPermitted Code = "not_permitted"
	// CodeBudgetExceeded: a resource bound was hit. Split the change.
	CodeBudgetExceeded Code = "budget_exceeded"
	// CodeUnreachable: the server or backend holding the content did not
	// answer. Retry later.
	CodeUnreachable Code = "unreachable"
)

// Codes returns every error code, in the order the contract lists them.
func Codes() []Code {
	return slices.Clone(allCodes)
}

var allCodes = []Code{
	CodeInvalid, CodeNotFound, CodeAmbiguous, CodeStale, CodeDocChanged, CodeGuard,
	CodeGateFailed, CodeUnsupported, CodeNotPermitted, CodeBudgetExceeded, CodeUnreachable,
}

// Exit codes a CLI returns for a refusal. They keep the meanings the kapi
// binary gives them (host/exitcode.go): 2 is a malformed invocation, 3 is
// "did not land; re-read and retry or change approach", 5 is an unreachable
// backend.
const (
	ExitInvalid     = 2
	ExitRefused     = 3
	ExitUnreachable = 5
)

// ExitCode is the process exit code a CLI returns when a change set is
// refused with c.
func (c Code) ExitCode() int {
	switch c {
	case CodeInvalid:
		return ExitInvalid
	case CodeUnreachable:
		return ExitUnreachable
	default:
		return ExitRefused
	}
}

// HTTPStatus is the status an HTTP transport answers a refusal with c.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeInvalid:
		return 400
	case CodeNotFound:
		return 404
	case CodeAmbiguous, CodeStale, CodeDocChanged:
		return 409
	case CodeGuard, CodeGateFailed, CodeUnsupported:
		return 422
	case CodeNotPermitted:
		return 403
	case CodeBudgetExceeded:
		return 413
	case CodeUnreachable:
		return 503
	default:
		return 500
	}
}

// Subcode narrows a guard refusal.
type Subcode string

const (
	// SubcodeCodesChanged: the result drops, repeats, reorders or unbalances an
	// inline code its constraints protect, or names a code the reference does
	// not hold.
	SubcodeCodesChanged Subcode = "codes_changed"
	// SubcodeStructureLost: the result would flatten a plural or select, or an
	// edit reaches into one without naming its branch with a path.
	SubcodeStructureLost Subcode = "structure_lost"
	// SubcodeBadPosition: a position is outside the sequence it addresses.
	SubcodeBadPosition Subcode = "bad_position"
	// SubcodeOverlap: two edits of one operation overlap.
	SubcodeOverlap Subcode = "overlap"
)

// Error is a refusal. Code is always set; the other fields carry what the code
// says it carries.
type Error struct {
	Code    Code    `json:"code"`
	Subcode Subcode `json:"subcode,omitempty"`
	// Field names the operation field at fault, as a path inside the
	// operation: "if_match", "basis", "edits/1/find".
	Field string `json:"field,omitempty"`
	// Pointer is the JSON pointer of what failed to decode, into the change
	// set as sent ("/ops/2/edits/0/find").
	Pointer string `json:"pointer,omitempty"`
	Message string `json:"message"`
	// Candidates are what an ambiguous reference or find matched, or what a
	// reference that resolved to nothing might have meant.
	Candidates []Candidate `json:"candidates,omitempty"`
	// Expected and Found describe a guard refusal.
	Expected string `json:"expected,omitempty"`
	Found    string `json:"found,omitempty"`
	// Capability names what an unsupported operation needs.
	Capability string `json:"capability,omitempty"`
}

// Error implements error.
func (e *Error) Error() string {
	switch {
	case e.Pointer != "":
		return fmt.Sprintf("%s at %s: %s", e.Code, e.Pointer, e.Message)
	case e.Subcode != "":
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Subcode, e.Message)
	default:
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
}

// Candidate is one thing a reference or a find could mean.
type Candidate struct {
	// Key is a block key or an edition key.
	Key string `json:"key,omitempty"`
	// Rev is the candidate's revision.
	Rev string `json:"rev,omitempty"`
	// Text is the first 80 characters of the candidate's content, or the text
	// around a match.
	Text string `json:"text,omitempty"`
	// Occurrence is the 1-based number of a find match, the value to send as
	// occurrence to choose it.
	Occurrence int `json:"occurrence,omitempty"`
	// At is where a find match lies.
	At *Resolved `json:"at,omitempty"`
}

func errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func invalidAt(pointer, format string, args ...any) *Error {
	return &Error{Code: CodeInvalid, Pointer: pointer, Message: fmt.Sprintf(format, args...)}
}

func guardf(sub Subcode, format string, args ...any) *Error {
	return &Error{Code: CodeGuard, Subcode: sub, Message: fmt.Sprintf(format, args...)}
}
