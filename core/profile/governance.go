package profile

import (
	"strconv"

	"github.com/neokapi/neokapi/core/tool"
)

// GovernanceContext resolves the provenance fields that record what governed a
// translation producer when it wrote a target: the profile's id, the revision
// pinned at production time, and a fingerprint of the rendered guidance plus the
// terminology in force. Every producer — AI, MT, recycled memory — stamps these
// onto model.Origin through this one function, so a target's governance stamp
// does not depend on which engine wrote it: two producers under one context
// yield the same fingerprint and fall stale together when that context moves.
//
// All three results are empty when the producer was ungoverned — no profile and
// no terminology — which reads correctly as an ad-hoc run.
func GovernanceContext(p *VoiceProfile, glossary map[string]string) (profileID, profileVersion, contextFP string) {
	if p != nil {
		profileID = p.ID
		if p.Version > 0 {
			profileVersion = strconv.Itoa(p.Version)
		}
	}
	contextFP = tool.ContextFingerprint(RenderVoiceGuideCompact(p), glossary)
	return
}
