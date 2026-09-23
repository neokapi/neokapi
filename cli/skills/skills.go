// Package skills carries the kapi agent skill into the binary.
//
// The source tree under data/ is the one the make targets publish to the
// plugin marketplace and to the portable collection. The binary uses it two
// ways. `kapi init` writes the skill's SKILL.md into a project, and nothing
// else: the skill is short and names four habits. `kapi help <topic>` serves
// the reference files beside it, so an agent reads the guidance of the binary
// it is running rather than a copy that lags it.
package skills

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing/fstest"
)

//go:embed data
var data embed.FS

//go:embed retired.sha256
var retiredList string

// skillName is the one skill the binary carries.
const skillName = "kapi"

// skillFile is the file kapi init writes for it.
const skillFile = "SKILL.md"

// Tree returns the whole embedded skill source: one directory per skill, each
// holding a SKILL.md and its references.
func Tree() fs.FS {
	sub, err := fs.Sub(data, "data")
	if err != nil {
		// data is embedded at build time, so a failure here is a broken build
		// rather than anything a caller did.
		panic("skills: embedded skill tree: " + err.Error())
	}
	return sub
}

// Wiring returns what `kapi init` writes into a project's skill directory: the
// kapi skill's SKILL.md and nothing else.
func Wiring() fs.FS {
	body, err := fs.ReadFile(Tree(), path.Join(skillName, skillFile))
	if err != nil {
		panic("skills: embedded SKILL.md: " + err.Error())
	}
	return fstest.MapFS{
		path.Join(skillName, skillFile): &fstest.MapFile{Data: body, Mode: 0o644},
	}
}

var (
	retiredOnce sync.Once
	retiredSet  map[string]bool
)

// Retired reports whether body is a file some kapi release copied into a
// project's skill directory: any file of the embedded skill tree, or one of
// the earlier versions listed in retired.sha256. `kapi init` removes such a
// file from a skill directory and keeps every other.
func Retired(body []byte) bool {
	retiredOnce.Do(func() {
		retiredSet = map[string]bool{}
		sc := bufio.NewScanner(strings.NewReader(retiredList))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				retiredSet[line] = true
			}
		}
		_ = fs.WalkDir(Tree(), skillName, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if b, rerr := fs.ReadFile(Tree(), p); rerr == nil {
				retiredSet[digest(b)] = true
			}
			return nil
		})
	})
	return retiredSet[digest(body)]
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Topic is one reference file served by `kapi help <topic>`.
type Topic struct {
	// Name is what `kapi help` takes: the file's stem, with `i18n-` in front
	// for the per-stack playbooks.
	Name string
	// Title is the file's first heading.
	Title string
	// Path is the file's path inside the skill.
	Path string
}

var (
	topicsOnce sync.Once
	topicList  []Topic
	topicByKey map[string]Topic
	topicByRef map[string]string // skill-relative path → topic name
)

func loadTopics() {
	topicsOnce.Do(func() {
		topicByKey = map[string]Topic{}
		topicByRef = map[string]string{}
		root := path.Join(skillName, "references")
		_ = fs.WalkDir(Tree(), root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel := strings.TrimPrefix(p, root+"/")
			ext := path.Ext(rel)
			if ext != ".md" && ext != ".yaml" {
				return nil
			}
			name := strings.ReplaceAll(strings.TrimSuffix(rel, ext), "/", "-")
			body, rerr := fs.ReadFile(Tree(), p)
			if rerr != nil {
				return nil
			}
			t := Topic{Name: name, Title: firstHeading(body, ext == ".yaml"), Path: p}
			topicList = append(topicList, t)
			topicByKey[name] = t
			topicByRef[p] = name
			return nil
		})
		sort.Slice(topicList, func(i, j int) bool { return topicList[i].Name < topicList[j].Name })
	})
}

// Topics lists every help topic, by name.
func Topics() []Topic {
	loadTopics()
	out := make([]Topic, len(topicList))
	copy(out, topicList)
	return out
}

// TopicText returns a topic's text for a terminal: the reference file, with
// each link to another reference rewritten as the `kapi help` command that
// serves it. ok is false for a name that is no topic.
func TopicText(name string) (string, bool) {
	loadTopics()
	t, ok := topicByKey[name]
	if !ok {
		return "", false
	}
	body, err := fs.ReadFile(Tree(), t.Path)
	if err != nil {
		return "", false
	}
	return rewriteLinks(string(body), path.Dir(t.Path)), true
}

// mdLink matches an inline Markdown link.
var mdLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

// rewriteLinks turns `[text](other.md)` into `text (kapi help other)` when
// other.md is a topic, and leaves every other link as written.
func rewriteLinks(body, dir string) string {
	return mdLink.ReplaceAllStringFunc(body, func(m string) string {
		parts := mdLink.FindStringSubmatch(m)
		target, _, _ := strings.Cut(parts[2], "#")
		name, ok := topicByRef[path.Clean(path.Join(dir, target))]
		if !ok {
			return m
		}
		cmd := "kapi help " + name
		if strings.Trim(parts[1], "`") == cmd || strings.HasSuffix(parts[1], ".md") || strings.HasSuffix(parts[1], ".yaml") {
			return "`" + cmd + "`"
		}
		return parts[1] + " (`" + cmd + "`)"
	})
}

// firstHeading returns the text of a file's first `# ` heading. For a YAML
// file, whose first comment line runs on after a colon, it is the part before.
func firstHeading(body []byte, yamlFile bool) string {
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if h, ok := strings.CutPrefix(line, "# "); ok {
			if before, _, found := strings.Cut(h, ":"); yamlFile && found {
				return strings.TrimSpace(before)
			}
			return strings.TrimSpace(h)
		}
	}
	return ""
}
