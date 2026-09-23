package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrVoiceAlreadyBound reports a voice binding that is already declared where
// BindVoice was asked to add one.
var ErrVoiceAlreadyBound = errors.New("recipe: a voice is already bound here")

// BindVoice declares `voice: {profile: <id>}` in the recipe at path: under
// `defaults:` when profile is empty, or under `profiles.<profile>:`.
//
// The edit is an insertion into the text, and every byte of the recipe outside
// the lines it adds stays as the author wrote it: comments, blank lines, key
// order, quoting, flow and block styles. A recipe is committed, human-authored
// source, and a binding added on someone's behalf should show in a diff as
// exactly the lines it adds.
//
// The two lines go directly under the parent key, ahead of its first child, at
// the indentation the parent's children already use. A recipe with no
// `defaults:` gains one at its end. A parent written in flow style (`defaults:
// {…}`) has no line to insert under, so that recipe is written back through
// Save instead, which keeps its comments and order but may restyle the parent.
//
// It returns ErrVoiceAlreadyBound when the parent already declares a `voice:`,
// whatever it binds: replacing a binding a person wrote is a different decision
// from adding one where there was none.
func BindVoice(path, profile, id string) error {
	if id == "" {
		return errors.New("recipe: a voice binding needs a profile id")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project file: %w", err)
	}
	out, ok, err := insertVoiceBinding(data, profile, id)
	if err != nil {
		return err
	}
	if !ok {
		return bindVoiceBySave(path, profile, id)
	}
	// The result must still be a recipe, and must bind what was asked.
	var check KapiProject
	if err := yaml.Unmarshal(out, &check); err != nil {
		return fmt.Errorf("recipe: binding the voice would leave an unreadable recipe: %w", err)
	}
	if got := voiceAt(&check, profile); got == nil || got.Profile != id {
		return bindVoiceBySave(path, profile, id)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat project file: %w", err)
	}
	if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}
	return nil
}

// NamedFile is the path a binding names when it is written in a form that names
// a file (`voice: <path>` or `profile_file:`), which a recipe does not load
// with. Empty for a binding by name.
func (b *VoiceBinding) NamedFile() string {
	if b == nil {
		return ""
	}
	return b.namedFile
}

// DeclaredDefaultVoice reads the voice binding the recipe at path declares
// under `defaults:`, nil when it declares none. The recipe is parsed and not
// validated, so a binding in a form the loader rejects is still reported, with
// NamedFile set.
func DeclaredDefaultVoice(path string) (*VoiceBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}
	var head struct {
		Defaults struct {
			Voice *VoiceBinding `yaml:"voice"`
		} `yaml:"defaults"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("parse project file: %w", err)
	}
	return head.Defaults.Voice, nil
}

// voiceAt is the voice binding a recipe declares under a parent: `defaults:`
// for an empty profile, else that profile.
func voiceAt(p *KapiProject, profile string) *VoiceBinding {
	if profile == "" {
		return p.Defaults.Voice
	}
	return p.Profiles[profile].Voice
}

// bindVoiceBySave is the fallback for a parent the text insertion cannot reach.
func bindVoiceBySave(path, profile, id string) error {
	proj, err := LoadWithOptions(path, LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	binding := &VoiceBinding{Profile: id}
	if profile == "" {
		proj.Defaults.Voice = binding
	} else {
		pr, ok := proj.Profiles[profile]
		if !ok {
			return fmt.Errorf("recipe: this project declares no profile %q", profile)
		}
		pr.Voice = binding
		proj.Profiles[profile] = pr
	}
	return Save(path, proj)
}

// insertVoiceBinding returns the recipe text with the binding inserted, and
// false when the parent is in a form the insertion does not handle.
func insertVoiceBinding(data []byte, profile, id string) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, fmt.Errorf("parse project file: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, false, errors.New("recipe: the project file is not a mapping")
	}
	root := doc.Content[0]
	scalar, err := yamlScalar(id)
	if err != nil {
		return nil, false, err
	}

	parentKey, parent := mappingValue(root, "defaults")
	if profile != "" {
		_, profiles := mappingValue(root, "profiles")
		if profiles == nil || profiles.Kind != yaml.MappingNode {
			return nil, false, fmt.Errorf("recipe: this project declares no profile %q", profile)
		}
		parentKey, parent = mappingValue(profiles, profile)
		if parentKey == nil {
			return nil, false, fmt.Errorf("recipe: this project declares no profile %q", profile)
		}
	}

	lines := splitLinesKeepEnds(data)
	if parentKey == nil {
		// No `defaults:` at all: the recipe gains one at its end.
		var b bytes.Buffer
		b.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "defaults:\n  voice:\n    profile: %s\n", scalar)
		return b.Bytes(), true, nil
	}

	keyIndent := parentKey.Column - 1
	var childIndent int
	switch {
	case parent == nil || (parent.Kind == yaml.ScalarNode && parent.Tag == "!!null" && parent.Value == ""):
		childIndent = keyIndent + 2
	case parent.Kind == yaml.MappingNode && parent.Style&yaml.FlowStyle == 0:
		if k, _ := mappingValue(parent, "voice"); k != nil {
			return nil, false, ErrVoiceAlreadyBound
		}
		if len(parent.Content) == 0 {
			childIndent = keyIndent + 2
		} else {
			childIndent = parent.Content[0].Column - 1
		}
	default:
		return nil, false, nil
	}
	step := childIndent - keyIndent
	if step <= 0 {
		step = 2
	}

	// The parent key sits on one line; the binding goes on the lines after it.
	at := parentKey.Line // 1-based line of the key, so lines[at:] follow it
	if at > len(lines) {
		return nil, false, nil
	}
	nl := "\n"
	if bytes.HasSuffix(lines[at-1], []byte("\r\n")) {
		nl = "\r\n"
	}
	insert := fmt.Sprintf("%svoice:%s%sprofile: %s%s",
		strings.Repeat(" ", childIndent), nl, strings.Repeat(" ", childIndent+step), scalar, nl)

	var b bytes.Buffer
	for _, l := range lines[:at] {
		b.Write(l)
	}
	if !bytes.HasSuffix(lines[at-1], []byte("\n")) {
		b.WriteString(nl)
	}
	b.WriteString(insert)
	for _, l := range lines[at:] {
		b.Write(l)
	}
	return b.Bytes(), true, nil
}

// mappingValue returns the key and value nodes for key in a mapping node.
func mappingValue(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// yamlScalar renders s as a one-line YAML scalar, quoted where it has to be.
func yamlScalar(s string) (string, error) {
	out, err := yaml.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("encode %q: %w", s, err)
	}
	v := strings.TrimRight(string(out), "\n")
	if strings.Contains(v, "\n") {
		return "", fmt.Errorf("recipe: a profile id must be one line, got %q", s)
	}
	return v, nil
}

// splitLinesKeepEnds splits data into lines, each keeping its line ending.
func splitLinesKeepEnds(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			lines = append(lines, data)
			break
		}
		lines = append(lines, data[:i+1])
		data = data[i+1:]
	}
	return lines
}
