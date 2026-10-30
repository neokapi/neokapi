package backend

// "Deeper look with AI" on the Map runs the deeper look `kapi context suggest
// --ai` runs. What the model proposes waits for a person under "What your
// files do", as suggestions by kapi named with the model.
//
// The run writes the context log, which a folder without a recipe has no home
// for. Asking for it acts on the project, as running a flow does, so once a
// model is known to answer, ensureRecipe writes the folder's kapi.yaml first.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/host"
)

// deeperLookTimeout bounds one deeper look. The model reads a bounded sample
// of the project, so a run is a fixed number of calls; this is the ceiling for
// a slow model.
const deeperLookTimeout = 10 * time.Minute

// DeeperLookModelDTO names the model a deeper look of a tab would ask.
type DeeperLookModelDTO struct {
	// Connected is false when nothing names a model: no provider in the
	// recipe or the app config, no saved credential and no API key in the
	// environment. The Map then offers to connect one.
	Connected bool   `json:"connected"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
}

// DeeperLookDTO is what one deeper look did.
type DeeperLookDTO struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Filed counts the suggestions this run recorded new.
	Filed int `json:"filed"`
	// Asked counts the blocks the model read, of Blocks.
	Asked  int `json:"asked"`
	Blocks int `json:"blocks"`
	// Tokens is the token count the provider reported, zero when it reported
	// none.
	Tokens int `json:"tokens"`
}

// deeperLookApp is a run-scoped host.App for the deeper look, with the
// desktop's AI defaulting and credential resolution, as a convergence run has.
func (a *App) deeperLookApp() *host.App {
	run := a.borrowEngine(&host.App{Credentials: a.credentials, Config: a.aiConfig, Quiet: true})
	run.InitRegistries()
	run.ToolReg.SetConfigPreprocessor(a.aiConfigPreprocessor())
	return run
}

// DeeperLookModel names the model a deeper look of the tab would ask, and
// whether there is one. It asks no model.
func (a *App) DeeperLookModel(tabID string) (*DeeperLookModelDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	// The answer is kept for the session, so showing the Map again reads no
	// saved key (lookmodel.go).
	model := a.lookModel(op.Project)
	return &model, nil
}

// DeeperLook reads the tab's source blocks with the connected model and files
// what it proposes as suggestions. With no model connected it refuses with
// host.ErrNoAIProvider, which the Map answers by offering to connect one.
func (a *App) DeeperLook(tabID string) (*DeeperLookDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.contextOnly || op.Path == "" {
		return nil, errors.New("this tab holds no files to read: open the project's folder")
	}
	// The refusal comes before anything is written: a folder without a
	// recipe gets one only for a run that will ask a model.
	run := a.deeperLookApp()
	if _, _, err := run.ResolveDeeperLookModel(op.Project); err != nil {
		// The Map asks again when this refusal reaches it, and the answer
		// it then gets is read from the keychain.
		a.forgetLookModels()
		return nil, err
	}
	if err := a.ensureRecipe(op); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), deeperLookTimeout)
	defer cancel()
	// The calls the run makes are listed in the session's AI activity under
	// the Map.
	ctx = withAIScope(ctx, AIActivityScope{Surface: "map", Action: "deeper-look"})

	res, err := run.DeeperLook(ctx, host.DeeperLookRequest{Project: op.Path})
	if err != nil {
		return nil, err
	}
	out := &DeeperLookDTO{
		Provider: res.Provider,
		Model:    res.Model,
		Filed:    res.FiledCount(),
		Asked:    res.Asked,
		Blocks:   res.Blocks,
	}
	if res.Usage != nil {
		out.Tokens = res.Usage.TotalTokens()
	}
	if res.ChangedCount() > 0 {
		// A row filed or filed again changes what the Map and Decide list.
		a.emitEvent("workspace:changed", nil)
	}
	return out, nil
}
