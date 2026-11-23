package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
)

// importCSVAs posts csv to the CSV import route as who, naming updateOf as
// the import it updates when set, and returns the status and response.
func importCSVAs(t *testing.T, h *kgHarness, who, file, csv, updateOf string) (int, importConceptsResponse) {
	t.Helper()
	body, err := json.Marshal(ImportCSVRequest{CSVContent: csv, SourceLocale: "en", TargetLocale: "nb", Source: file, UpdateOf: updateOf})
	require.NoError(t, err)
	c, rec := h.req(http.MethodPost, "/", string(body), platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleImportConceptsCSV(withActor(c, who)))
	var out importConceptsResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return rec.Code, out
}

// Each import is one the server mints. Two people importing files with the
// same name are two imports, and the second changes nothing the first wrote;
// the first person updates their own import by naming it, chosen from the
// list of earlier imports.
func TestConceptImport_UpdatesOnlyTheImportItNames(t *testing.T) {
	h := newKGHarness(t)

	status, ann := importCSVAs(t, h, "ann", "terms.csv", "Save,Lagre\n", "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, ann.Created)
	require.NotEmpty(t, ann.Import)

	status, bob := importCSVAs(t, h, "bob", "terms.csv", "Save,Spare\n", "")
	require.Equal(t, http.StatusOK, status)
	assert.NotEqual(t, ann.Import, bob.Import, "the same file name is a different import")
	assert.Zero(t, bob.Imported)
	require.Len(t, bob.Conflicts, 1)
	assert.Equal(t, "Lagre", storedConcept(t, h.tb(t), "term:en:save").Terms[1].Text)

	c, rec := h.req(http.MethodGet, "/", "", platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleListConceptImports(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var imports []sqlterms.ImportRecord
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &imports))
	require.Len(t, imports, 1)
	assert.Equal(t, sqlterms.ImportRecord{ID: ann.Import, Name: "terms.csv", By: "ann", At: imports[0].At, Concepts: 1}, imports[0])

	status, update := importCSVAs(t, h, "ann", "terms.csv", "Save,Lagre data\n", ann.Import)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, update.Updated)
	assert.Equal(t, ann.Import, update.Import)
	assert.Equal(t, "Lagre data", storedConcept(t, h.tb(t), "term:en:save").Terms[1].Text)

	status, _ = importCSVAs(t, h, "bob", "terms.csv", "Save,Spare\n", "import-nobody-made")
	assert.Equal(t, http.StatusNotFound, status, "an update must name an import the workspace holds")
}
