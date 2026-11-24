package project

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/rootpath"
)

// A checkout's record names the project its state belongs to, so state kept
// for one project is never taken into a checkout of another. The project is
// the recipe's id: `kapi init` mints it, renaming the project or moving its
// folder leaves it, and two checkouts of one repository share it, which is
// what lets a relink bring a checkout's own state across. The name is kept
// beside it for a person to read, and the plan shows the servers the sync
// state was issued by.

// checkoutProjectOf reads the project the layout's recipe states, decoding
// only its id and name, so a recipe that would not validate still names its
// project. ok is false when the recipe cannot be read.
func checkoutProjectOf(l Layout) (dataroot.CheckoutProject, bool) {
	data, err := rootpath.ReadFile(l.Root, l.RecipePath)
	if err != nil {
		return dataroot.CheckoutProject{}, false
	}
	var doc struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return dataroot.CheckoutProject{}, false
	}
	return dataroot.CheckoutProject{ID: displaySafe(doc.ID), Name: displaySafe(doc.Name)}, true
}

// recordCheckoutProject records, in the record of the checkout's state
// directory dir, the project the layout's recipe states now.
func recordCheckoutProject(l Layout, dir string) {
	if p, ok := checkoutProjectOf(l); ok {
		_ = dataroot.SetRecordProject(dir, p)
	}
}

// describeProject names a project for a person: its name and id where the
// record has them.
func describeProject(p *dataroot.CheckoutProject) string {
	switch {
	case p == nil || (p.ID == "" && p.Name == ""):
		return "a project kapi did not record"
	case p.ID == "":
		return fmt.Sprintf("%q", p.Name)
	case p.Name == "":
		return p.ID
	default:
		return fmt.Sprintf("%q (%s)", p.Name, p.ID)
	}
}

// OtherProjectAt names the project of state recorded for another directory at
// the layout's location when it is a different project from the layout's, and
// returns "" otherwise. Such state is never offered for a relink.
func OtherProjectAt(l Layout) string {
	path := dataroot.NormalizePath(l.Root)
	current, _ := checkoutProjectOf(l)
	for _, c := range dataroot.Checkouts() {
		if c.Record.Path != path || filepath.Clean(c.Dir) == filepath.Clean(l.LocalStateDir()) {
			continue
		}
		if _, err := relinkVerdict(c.Record.Project, current); err != nil {
			return describeProject(c.Record.Project)
		}
	}
	return ""
}

// relinkVerdict compares the project an earlier checkout's state was recorded
// for with the project of the checkout taking it. It returns an error naming
// both when they are different projects, and verified false when the record
// cannot say: a record written before kapi recorded the project, or a recipe
// on either side with no id.
//
// Two ids that differ are different projects. An id recorded where the recipe
// now has none is too, since a project does not drop its id. Two recipes with
// no id and different names are different projects, because the name is then
// what identifies the project (KapiProject.Identity).
func relinkVerdict(recorded *dataroot.CheckoutProject, current dataroot.CheckoutProject) (verified bool, err error) {
	if recorded == nil {
		return false, nil
	}
	differ := func() error {
		return fmt.Errorf("project: the state recorded at this location belongs to %s, and this checkout is %s; kapi does not take one project's state into another",
			describeProject(recorded), describeProject(&current))
	}
	switch {
	case recorded.ID != "" && current.ID != "":
		if recorded.ID != current.ID {
			return false, differ()
		}
		return true, nil
	case recorded.ID != "" && current.ID == "":
		return false, differ()
	case recorded.ID == "" && current.ID == "" && recorded.Name != "" && current.Name != "" && recorded.Name != current.Name:
		return false, differ()
	default:
		return false, nil
	}
}
