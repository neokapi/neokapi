package output

import (
	"bytes"
	"testing"
)

func TestWorkspaceListOutput_FormatText(t *testing.T) {
	tests := []struct {
		name string
		out  WorkspaceListOutput
		want string
	}{
		{
			name: "empty",
			out:  WorkspaceListOutput{},
			want: "No workspaces found.\n",
		},
		{
			name: "personal and team",
			out: WorkspaceListOutput{
				Workspaces: []WorkspaceItem{
					{Slug: "alice", Name: "Alice", Type: "personal"},
					{Slug: "acme", Name: "Acme Corp"},
					{Slug: "solo", Name: "solo"},
				},
			},
			want: "alice (Alice) [personal]\nacme (Acme Corp)\nsolo\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tt.out.FormatText(&buf); err != nil {
				t.Fatalf("FormatText: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("FormatText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkspaceCreateOutput_FormatText(t *testing.T) {
	tests := []struct {
		name string
		out  WorkspaceCreateOutput
		want string
	}{
		{
			name: "name differs from slug",
			out:  WorkspaceCreateOutput{Name: "Demo Workspace", Slug: "demo-workspace"},
			want: "Workspace created: demo-workspace (Demo Workspace)\n",
		},
		{
			name: "name equals slug",
			out:  WorkspaceCreateOutput{Name: "demo", Slug: "demo"},
			want: "Workspace created: demo\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tt.out.FormatText(&buf); err != nil {
				t.Fatalf("FormatText: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("FormatText() = %q, want %q", got, tt.want)
			}
		})
	}
}
