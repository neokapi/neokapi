package main

// allowProject lists the places in its area that may reach a file by path.
var allowProject = map[string]string{
	// The recipe file itself.
	"core/project/project.go:LoadWithOptions":         "the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/contextbackend.go:RecipeVenue":      "the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/contextbackend.go:ReadContextVenue": "the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/voicebind.go:BindVoice":             "the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/voicebind.go:DeclaredDefaultVoice":  "the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/extraction.go:ExtractionInputsHash": "reads the layout's RecipePath, the recipe file at the path the caller resolved: one a person named with -p or KAPI_PROJECT, one an MCP call named that ResolveMCPCallProject resolved within the server's roots, or one discovery found, which checkFoundRecipe refuses when it is a link out of its directory or not a regular file",
	"core/project/recipepath.go:namedRecipe":          "a project location a person named with -p or KAPI_PROJECT, or the recipe an MCP call named, already resolved within the server's roots by ResolveMCPCallProject; discovery never comes here (RecipeIn's callers are namedRecipe and ResolveNewRecipePath)",
	"core/project/recipepath.go:RecipeIn":             "a project location a person named with -p or KAPI_PROJECT, or the recipe an MCP call named, already resolved within the server's roots by ResolveMCPCallProject; discovery never comes here (RecipeIn's callers are namedRecipe and ResolveNewRecipePath)",
	"core/project/recipepath.go:followRecipeLink":     "follows a recipe link only for a project location a person named with -p or KAPI_PROJECT, or the recipe an MCP call named, already resolved within the server's roots by ResolveMCPCallProject; discovery never comes here (RecipeIn's callers are namedRecipe and ResolveNewRecipePath)",
	"core/project/recipepath.go:isRecipe":             "a project location a person named with -p or KAPI_PROJECT, or the recipe an MCP call named, already resolved within the server's roots by ResolveMCPCallProject; discovery never comes here (RecipeIn's callers are namedRecipe and ResolveNewRecipePath)",
	"core/project/layout.go:LayoutFor":                "stats the recipe path the caller resolved (ResolveRecipePath or ResolveMCPCallProject); the layout it returns has passed Layout.CheckState",
	"core/project/layout.go:ResolveLayout":            "stats the start directory a caller names (the working directory, a file a person named, an MCP path within the server's roots) to walk up from it; a found recipe goes through checkFoundRecipe",
	"core/project/layout.go:layoutAtDir":              "lists the start directory of discovery and its ancestors, reading only entry names; a found recipe that is not a regular file inside its directory is refused (checkFoundRecipe)",
	"core/project/statecheck.go:CheckState":           "the check itself: it lstats the state directory and walks it (WalkDir follows no link) to refuse a link or irregular file beneath it",
	"core/project/statecheck.go:gitIndexStamp":        "stats the index of the repository findGitDir found, to tell whether git must be asked again; it reads nothing",
	"core/project/statecheck.go:findGitDir":           "walks up from the project root to the repository holding it; of a .git pointer file it reads only the gitdir line, and it reads or writes nothing that line names",

	// The project state directory.
	"core/project/layout.go:EnsureLocalDir":             "creates a named subdirectory of the checkout's state directory under the data root (Layout.LocalStateDir), which kapi creates and no checkout supplies",
	"core/project/layout.go:EnsureLayout":               "creates the project root when a scaffold names one that is not there yet, and work/cache in the checkout's state directory under the data root (dataroot.EnsureCheckoutDir), which no checkout supplies; nothing is written in the checkout",
	"core/project/extraction.go:EnsureExtractionDir":    "creates a batch directory named by a checked batch ID (checkBatchID) under work/cache/extractions in the checkout's state directory under the data root, which no checkout supplies",
	"core/project/extraction.go:SaveExtractionManifest": "writes the manifest into the batch directory EnsureExtractionDir created in the checkout's state directory under the data root, which no checkout supplies",

	// A pattern a person typed.
	"core/project/glob.go:readDirAsNamed": "a directory a command-line pattern reaches (GlobOptions.LinksAnywhere, set only by host/inputs.go expandGlobArg)",
	"core/project/glob.go:statAsNamed":    "an entry a command-line pattern reaches (GlobOptions.LinksAnywhere, set only by host/inputs.go expandGlobArg)",
}
