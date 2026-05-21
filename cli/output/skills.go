package output

import (
	"fmt"
	"io"
)

// SkillEntry is one row in `kapi skills list`.
type SkillEntry struct {
	Name        string `json:"name"`
	Family      string `json:"family"`
	Description string `json:"description"`
}

// SkillsListOutput is the result of `kapi skills list`.
type SkillsListOutput struct {
	Skills []SkillEntry `json:"skills"`
	Total  int          `json:"total"`
}

// FormatText prints a grouped skill list.
func (o SkillsListOutput) FormatText(w io.Writer) error {
	for _, fam := range []string{"kapi", "bowrain"} {
		first := true
		for _, s := range o.Skills {
			if s.Family != fam {
				continue
			}
			if first {
				fmt.Fprintf(w, "\n%s skills:\n", fam)
				first = false
			}
			fmt.Fprintf(w, "  %-26s %s\n", s.Name, truncate(s.Description, 80))
		}
	}
	fmt.Fprintf(w, "\n%d skill(s). Install with: kapi skills install\n", o.Total)
	return nil
}

// SkillsInstallOutput is the result of install/uninstall/export.
type SkillsInstallOutput struct {
	Target    string   `json:"target"`
	Dir       string   `json:"dir"`
	Installed []string `json:"installed"`
	Total     int      `json:"total"`
}

// FormatText confirms what was written.
func (o SkillsInstallOutput) FormatText(w io.Writer) error {
	for _, p := range o.Installed {
		fmt.Fprintf(w, "  %s\n", p)
	}
	fmt.Fprintf(w, "%d skill(s) → %s\n", o.Total, o.Dir)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
