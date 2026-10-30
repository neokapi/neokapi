package source

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// kapimartTemplates is the kapimart sample's email templates. The HTML reader
// numbers its blocks differently with a skeleton wired: the comments and
// whitespace between the template sections, and the tables written without a
// tbody.
const kapimartTemplates = "../../../apps/kapi-desktop/backend/sample/kapimart/src/en/email-templates.html"

// jsxGuideMDX has a JSX element whose children are block-level markdown, which
// the MDX reader reads as markdown blocks only with a skeleton wired.
const jsxGuideMDX = `import { Callout } from "@site/components";

# Guide

<Callout type="note">

Keep the **recipe** beside the files it reads.

- Open the project folder.
- Check every file.

</Callout>
`

// TestPushReadsTheBlocksACheckReads: the blocks a push reads and sends, and
// the block index it sends beside them, are the blocks a check reports, by id
// and text. Pull writes with the skeleton wired and matches its blocks by
// name, so a push that read any other way would leave the venue holding blocks
// no pulled translation lands on.
func TestPushReadsTheBlocksACheckReads(t *testing.T) {
	templates, err := os.ReadFile(kapimartTemplates)
	require.NoError(t, err)

	srv := newScopeServer(t, "proj1", nil)
	conn := fileConnector(t, srv, []coreproj.Collection{
		{Name: "email", Content: []coreproj.ContentItem{{Path: "email/*.html"}}},
		{Name: "docs", Content: []coreproj.ContentItem{{Path: "docs/*.mdx"}}},
	}, map[string]string{
		"email/email-templates.html": string(templates),
		"docs/guide.mdx":             jsxGuideMDX,
	})

	checker := &host.App{}
	checker.InitRegistries()
	t.Cleanup(checker.Shutdown)

	files := map[string]string{"email/email-templates.html": "html", "docs/guide.mdx": "mdx"}
	checked := map[string][]*model.Block{}
	for rel, format := range files {
		abs := filepath.Join(conn.project.Root, filepath.FromSlash(rel))
		blocks, err := checker.ReadBlocksForCheck(t.Context(), abs, format, nil, "en")
		require.NoError(t, err)
		require.NotEmpty(t, blocks, rel)
		checked[rel] = blocks

		read, err := conn.readBlocks(t.Context(), abs, format)
		require.NoError(t, err)
		assert.Equal(t, idsAndTexts(checked[rel]), idsAndTexts(translatable(read)), "%s: the blocks push reads", rel)
	}

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Len(t, srv.commits, 1)
	commit := srv.commits[0]

	var items []apiclient.ItemMeta
	require.NoError(t, json.Unmarshal(commit.Items, &items))
	indexed := map[string]map[string]string{}
	for _, it := range items {
		var index editor.BlockIndex
		require.NoError(t, json.Unmarshal([]byte(it.BlockIndex), &index), it.Name)
		ids := map[string]string{}
		for _, b := range index.Blocks {
			if b.Translatable {
				ids[b.ID] = b.Source
			}
		}
		indexed[it.Name] = ids
	}

	for rel := range files {
		var hashes []string
		for _, b := range checked[rel] {
			hashes = append(hashes, model.ComputeContentHash(b.SourceText()))
		}
		tree, ok := commit.Tree[rel]
		require.True(t, ok, "%s: in the declared tree", rel)
		assert.ElementsMatch(t, hashes, tree.Content, "%s: the block texts the push declares", rel)

		require.Contains(t, indexed, rel)
		want := map[string]string{}
		for _, b := range checked[rel] {
			want[b.ID] = b.SourceText()
		}
		assert.Equal(t, want, indexed[rel], "%s: the block index the push sends", rel)
	}
}

func translatable(blocks []*model.Block) []*model.Block {
	var out []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

func idsAndTexts(blocks []*model.Block) map[string]string {
	out := make(map[string]string, len(blocks))
	for _, b := range blocks {
		out[b.ID] = model.RunsEditText(b.SourceRuns())
	}
	return out
}
