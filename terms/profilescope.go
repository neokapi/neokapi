package terms

// PropProfile is the concept property naming the profile a concept is scoped
// to: the point a rule's evidence was seen at, or the profile whose voice or
// terms file it was imported from. A concept without it holds across the whole
// project.
const PropProfile = "profile"

// Profile is the profile a concept is scoped to, empty for one that holds
// across the whole project.
func (c Concept) Profile() string { return c.Properties[PropProfile] }

// ScopeToProfile scopes a concept to a profile. An empty profile leaves it
// holding across the whole project.
func (c *Concept) ScopeToProfile(profile string) {
	if profile == "" {
		return
	}
	if c.Properties == nil {
		c.Properties = map[string]string{}
	}
	c.Properties[PropProfile] = profile
}

// AtProfile keeps the concepts that hold where a profile governs: the ones
// scoped to no profile, and the ones scoped to that profile. An empty profile
// is the project's default point, where only unscoped concepts hold.
func AtProfile(concepts []Concept, profile string) []Concept {
	out := make([]Concept, 0, len(concepts))
	for _, c := range concepts {
		if p := c.Profile(); p == "" || p == profile {
			out = append(out, c)
		}
	}
	return out
}
