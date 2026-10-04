package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
)

// FormatOpsOutput is what kapi formats --ops prints: what each format takes
// of the change contract, as describe_format answers for one format. One
// format named prints its description alone; none prints every format kapi
// can edit.
type FormatOpsOutput struct {
	Formats []change.Description
	// One says a single format was named.
	One bool
}

// MarshalJSON writes the one description named, or the list.
func (o FormatOpsOutput) MarshalJSON() ([]byte, error) {
	if o.One && len(o.Formats) == 1 {
		return json.Marshal(o.Formats[0])
	}
	if o.Formats == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(o.Formats)
}

// FormatText writes one line per format: its name, where it keeps editions,
// and the operations it takes.
func (o FormatOpsOutput) FormatText(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FORMAT\tEDITIONS\tOPERATIONS")
	for _, d := range o.Formats {
		ops := make([]string, 0, 9)
		for _, k := range d.Supported() {
			ops = append(ops, string(k))
		}
		for _, n := range d.Native {
			ops = append(ops, n.Name)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Format, d.Editions, strings.Join(ops, ", "))
	}
	return tw.Flush()
}

// DescribeFormatOps describes what each named format takes of the change
// contract, or every format kapi can edit when none is named: the description
// describe_format and section 2.7 of the edit model give, which kapi apply
// refuses anything outside of. An unknown name is a not_found error.
func (a *App) DescribeFormatOps(ctx context.Context, names []string) (FormatOpsOutput, error) {
	a.InitRegistries()
	svc := change.NewService(filehome.Formats{Registry: a.FormatReg}, nil)
	out := FormatOpsOutput{One: len(names) == 1}
	if len(names) == 0 {
		for _, info := range a.FormatReg.FormatInfos() {
			if !info.Editable && !info.Interchange {
				continue
			}
			name := string(info.Name)
			if strings.Contains(name, "@") {
				// A versioned entry beside its bare name describes the same
				// writer.
				continue
			}
			names = append(names, name)
		}
		slices.Sort(names)
	}
	for _, name := range names {
		d, err := svc.Describe(ctx, change.DescribeRequest{Format: name})
		if err != nil {
			return FormatOpsOutput{}, err
		}
		out.Formats = append(out.Formats, *d)
	}
	return out, nil
}
