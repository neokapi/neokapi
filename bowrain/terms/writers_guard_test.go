package terms_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The kinds of concept writer, by what each does when another write lands
// between its read and its write.
const (
	// writerPerson writes a person's edit made on what they read: it names
	// that revision and is refused (409) when the concept moved.
	writerPerson = "person"
	// writerDerives asks for an outcome, not a value: it reads again and
	// derives again (sqlterms.Rederive, sqlterms.Rewrite).
	writerDerives = "derives"
	// writerCreates writes a concept under a new id, or one it cleared
	// first; AddConcept refuses to replace a stored one.
	writerCreates = "creates"
	// writerImports writes a file's rows (sqlterms.ImportRows).
	writerImports = "imports"
	// writerCopy writes a private in-memory store no one else reads.
	writerCopy = "copy"
)

// The kinds of concept deleter. Every delete names the revision its caller
// read and is decided in the statement that deletes (DeleteConceptAt,
// DeleteConceptsAt); they differ in where that revision comes from.
const (
	// deletePerson deletes what a person decided on, at the revision read
	// for the decision, and is refused (409) when the concept moved.
	deletePerson = "person"
	// deleteProposed deletes at the revision recorded when the deletion was
	// proposed (knowledge.PinDeleteRevision); a concept changed since is a
	// merge conflict carrying the concept as it stands.
	deleteProposed = "proposed"
	// deleteShadow deletes a pilot's shadow copies at the revisions read,
	// all or none.
	deleteShadow = "shadow"
)

// conceptWriter is one function that writes a concept on the server side:
// its kind, and the test that puts it in the race its kind must win or lose.
type conceptWriter struct {
	kind string
	test string
}

// conceptWriters lists every function under bowrain/ (outside the store
// itself, the client plugin and the desktop app, which write their own local
// stores) that writes a concept. TestEveryConceptWriterIsListed walks the
// source and fails on a writer missing here, so a new one cannot be added
// without saying what it does in the race, and naming the test that shows it.
var conceptWriters = map[string]conceptWriter{
	"server.(*Server).HandleCreateConcept": {writerCreates, "TestConceptAPI_DoNotTranslateIsGoverned"},
	"server.(*Server).HandleUpdateConcept": {writerPerson, "TestConceptWriters_APersonsUpdateNamesTheRevisionItRead"},
	"server.(*Server).HandleWidenConcept":  {writerDerives, "TestConceptWriters_WideningKeepsAnEditMadeMeanwhile"},
	"server.(*Server).keepCandidate":       {writerPerson, "TestConceptWriters_ADeckDecisionActsAtTheDealtRevision"},
	"server.(*Server).decideTermTranslation": {writerDerives,
		"TestConceptWriters_ATranslationDecisionKeepsAnEditMadeMeanwhile"},
	"server.(*Server).promoteRuleToTerms":     {writerDerives, "TestConceptWriters_PromotingARuleKeepsAnEditMadeMeanwhile"},
	"server.(*Server).demoteRuleFromTerms":    {writerDerives, "TestConceptWriters_PromotingARuleKeepsAnEditMadeMeanwhile"},
	"server.(*Server).landWordRules":          {writerDerives, "TestConceptWriters_LandingWordRulesKeepsAnEditMadeMeanwhile"},
	"server.(*Server).importConcepts":         {writerImports, "TestImportRows"},
	"server.(*Server).promoteEntityToConcept": {writerCreates, "TestConceptAPI_DoNotTranslateIsGoverned"},
	"server.(*Server).HandleApproveContextScan": {writerCreates,
		"TestConceptAPI_DoNotTranslateIsGoverned"},
	"server.readTermSnapshot":                     {writerCopy, "TestEveryConceptWriterIsListed"},
	"mcp.(*MCPServer).handleTermAdd":              {writerCreates, "TestConceptAPI_DoNotTranslateIsGoverned"},
	"mcp.(*MCPServer).handlePromoteRule":          {writerDerives, "TestConceptWriters_PromotingARuleKeepsAnEditMadeMeanwhile"},
	"jobs.fileTermForms":                          {writerDerives, "TestFileTermForms_KeepsAnEditMadeWhileItRan"},
	"jobs.fileTermProposals":                      {writerDerives, "TestFileTermProposals_KeepsAnEditMadeWhileItRan"},
	"termcandidate.File":                          {writerDerives, "TestConceptWriters_RefreshingACandidateKeepsAnEditMadeMeanwhile"},
	"termspush.Apply":                             {writerPerson, "TestTermsPush_AnEditLandsOnlyOnTheConceptItRead"},
	"termscope.(*storeView).AddConceptWithStream": {writerCreates, "TestEveryConceptWriterIsListed"},
	"knowledge.editConcept":                       {writerDerives, "TestMergeChangeSet_KeepsAnEditMadeMeanwhile"},
	"knowledge.applyTermsOp":                      {writerCreates, "TestMergeChangeSet_KeepsAnEditMadeMeanwhile"},
	"knowledge.(*Engine).replacePilotShadow":      {writerCreates, "TestEveryConceptWriterIsListed"},
	"knowledge.cloneInMemoryTerms":                {writerCopy, "TestEveryConceptWriterIsListed"},
	"knowledge.(*Engine).seedConcept":             {writerCopy, "TestEveryConceptWriterIsListed"},
	"knowledge.(*Engine).ConceptUsage":            {writerCopy, "TestEveryConceptWriterIsListed"},
	"server.(*Server).recheckConceptViolations":   {writerCopy, "TestEveryConceptWriterIsListed"},
}

// conceptDeleters lists every function under the same trees that deletes a
// concept, with its kind and the test that puts it in the race. A framework
// routine run through sqlterms.Rewrite (fw.DemoteRule removing a concept's
// last term) deletes through the adapter Rewrite hands it, at the revision
// the routine read; TestRewrite_ADeleteNamesTheRevisionItRead covers it, and
// its callers are listed in conceptWriters as deriving.
var conceptDeleters = map[string]conceptWriter{
	"server.(*Server).forgetCandidate": {deletePerson, "TestConceptWriters_ADeckDecisionActsAtTheDealtRevision"},
	"knowledge.applyTermsOp":           {deleteProposed, "TestMergeChangeSet_DeletesOnlyTheConceptItWasProposedFor"},
	"knowledge.(Pilot).clearShadow":    {deleteShadow, "TestStopPilot_KeepsAShadowEditedMeanwhile"},
}

// deleteCalls are the calls that delete a concept.
var deleteCalls = map[string]bool{"DeleteConcept": true, "DeleteConceptAt": true, "DeleteConceptsAt": true}

// writeCalls are the calls that write a concept.
var writeCalls = map[string]bool{
	"AddConcept": true, "AddConceptWithStream": true, "UpdateConcept": true, "CreateConcept": true,
	"Rederive": true, "Rewrite": true, "ImportRows": true,
	"PromoteRule": true, "DemoteRule": true, "ImportCSV": true, "ImportJSON": true, "ImportTBX": true,
}

// skipped are the trees that write no workspace concept: the store itself,
// and clients that write their own local stores.
var skipped = []string{"terms", "plugin", "apps", "editorclient", "testutil"}

func TestEveryConceptWriterIsListed(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	found := map[string]bool{}
	deletes := map[string]bool{}
	tests := map[string]bool{}
	testFunc := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if slices.Contains(skipped, rel) || strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			src, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			for _, m := range testFunc.FindAllStringSubmatch(string(src), -1) {
				tests[m[1]] = true
			}
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := file.Name.Name + "." + funcName(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					found[name] = found[name] || writeCalls[sel.Sel.Name]
					deletes[name] = deletes[name] || deleteCalls[sel.Sel.Name]
				}
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)
	// The bowrain/terms tests are walked too, for the tests listed here.
	for _, extra := range []string{"terms"} {
		_ = filepath.WalkDir(filepath.Join(root, extra), func(path string, d fs.DirEntry, err error) error {
			if err == nil && strings.HasSuffix(path, "_test.go") {
				src, _ := os.ReadFile(path)
				for _, m := range testFunc.FindAllStringSubmatch(string(src), -1) {
					tests[m[1]] = true
				}
			}
			return nil
		})
	}

	check := func(listName string, calls map[string]bool, list map[string]conceptWriter) {
		var missing, stale, untested []string
		for name, writes := range calls {
			if _, ok := list[name]; writes && !ok {
				missing = append(missing, name)
			}
		}
		for name, w := range list {
			if !calls[name] {
				stale = append(stale, name)
			}
			if !tests[w.test] {
				untested = append(untested, name+" names "+w.test)
			}
		}
		slices.Sort(missing)
		slices.Sort(stale)
		slices.Sort(untested)
		assert.Empty(t, missing, "these functions are not in %s: say what each does when another write lands first, and name its test", listName)
		assert.Empty(t, stale, "these are listed in %s and no longer make such a call", listName)
		assert.Empty(t, untested, "these name a test that does not exist")
	}
	check("conceptWriters", found, conceptWriters)
	check("conceptDeleters", deletes, conceptDeleters)
}

// funcName is fn's name as conceptWriters spells it: Name, or (*Recv).Name.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	star := ""
	if s, ok := recv.(*ast.StarExpr); ok {
		star, recv = "*", s.X
	}
	if idx, ok := recv.(*ast.IndexExpr); ok {
		recv = idx.X
	}
	ident, _ := recv.(*ast.Ident)
	if ident == nil {
		return fn.Name.Name
	}
	return "(" + star + ident.Name + ")." + fn.Name.Name
}
