package screens

// The step ids, in the order the first project takes them. They match the
// steps of "Your first project" in the kapi docs.
const (
	StepDecide  = "decide"
	StepCheck   = "check"
	StepFix     = "fix"
	StepConnect = "connect"
	StepShare   = "share"
)

// MapStepDTO is one step of a first project: what it is, whether it is done,
// and the count that says how much of it is left.
type MapStepDTO struct {
	ID   string `json:"id"`
	Done bool   `json:"done"`
	// Count is what is left: the items awaiting a decision for decide, the
	// failing findings for fix. Zero for a step that counts nothing.
	Count int `json:"count,omitempty"`
}

// MapStepState is what MapSteps reads.
type MapStepState struct {
	// Awaiting counts the items awaiting a decision.
	Awaiting int
	// Checked reports a checks run over the whole project, and Failing the
	// findings of that run that fail.
	Checked bool
	Failing int
	// Connected reports an assistant connected to the project.
	Connected bool
	// Shared reports a recipe that shares the rules beyond this machine.
	Shared bool
}

// MapSteps lists the first project's steps with what each has left. Decide,
// connect and share stay done once done; check and fix follow each run. The
// list is empty when the three that stay done are done, so an established
// project draws none.
func MapSteps(s MapStepState) []MapStepDTO {
	decided := s.Awaiting == 0
	if decided && s.Connected && s.Shared {
		return nil
	}
	return []MapStepDTO{
		{ID: StepDecide, Done: decided, Count: s.Awaiting},
		{ID: StepCheck, Done: s.Checked},
		{ID: StepFix, Done: s.Checked && s.Failing == 0, Count: s.Failing},
		{ID: StepConnect, Done: s.Connected},
		{ID: StepShare, Done: s.Shared},
	}
}
