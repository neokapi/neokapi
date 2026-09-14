package event

// QualityGateGroupKey is the notification group a quality gate event belongs
// to: one gate, for one language, on one stream of one project. A failure
// notification carries it, and a pass for the same gate marks the notifications
// that share it as read. Empty for an event that does not name a project, gate
// and language.
func QualityGateGroupKey(ev Event) string {
	gate, locale := ev.Data["gate_name"], ev.Data["locale"]
	if ev.ProjectID == "" || gate == "" || locale == "" {
		return ""
	}
	return "quality-gate:" + ev.ProjectID + ":" + ev.Data["stream"] + ":" + gate + ":" + locale
}
