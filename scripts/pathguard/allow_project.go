package main

// allowProject lists the places in its area that may reach a file by path.
var allowProject = map[string]string{
	// The recipe file itself.
	"core/project/project.go:LoadWithOptions":         "the recipe file, at the path a person named with -p or that ResolveLayout found",
	"core/project/contextbackend.go:RecipeVenue":      "the recipe file, at the path a person named with -p or that ResolveLayout found",
	"core/project/contextbackend.go:ReadContextVenue": "the recipe file, at the path a person named with -p or that ResolveLayout found",
	"core/project/voicebind.go:BindVoice":             "the recipe file, at the path a person named with -p or that ResolveLayout found",
	"core/project/voicebind.go:DeclaredDefaultVoice":  "the recipe file, at the path a person named with -p or that ResolveLayout found",
	"core/project/extraction.go:ExtractionInputsHash": "the recipe file, at the layout's RecipePath, which a person named with -p or ResolveLayout found",
	"core/project/recipepath.go:namedRecipe":          "a project location a person named with -p or KAPI_PROJECT",
	"core/project/recipepath.go:RecipeIn":             "a project location a person named with -p or KAPI_PROJECT",
	"core/project/recipepath.go:followRecipeLink":     "a project location a person named with -p or KAPI_PROJECT",
	"core/project/recipepath.go:isRecipe":             "a project location a person named with -p or KAPI_PROJECT",
	"core/project/layout.go:LayoutFor":                "a recipe path a person named with -p; the layout it returns goes through Layout.CheckState",
	"core/project/layout.go:ResolveLayout":            "the working directory and its ancestors, which project discovery walks up from where a person ran kapi",
	"core/project/layout.go:layoutAtDir":              "the working directory and its ancestors, which project discovery walks up from where a person ran kapi",
	"core/project/statecheck.go:CheckState":           "the check itself: it lstats the state directory and its subdirectories to refuse one that is a link",

	// The project state directory.
	"core/project/layout.go:EnsureLocalDir":              "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/layout.go:EnsureLayout":                "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/layout.go:cacheOnly":                   "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/extraction.go:EnsureExtractionDir":     "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/extraction.go:SaveExtractionManifest":  "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/extraction.go:LoadExtractionManifest":  "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",
	"core/project/extraction.go:ListExtractionManifests": "kapi's own file in the project state directory, which Layout.CheckState keeps inside the project",

	// A pattern a person typed.
	"core/project/glob.go:readDirAsNamed": "a directory a pattern a person typed reaches (GlobOptions.LinksAnywhere, set only for an --input pattern)",
	"core/project/glob.go:statAsNamed":    "an entry a pattern a person typed reaches (GlobOptions.LinksAnywhere, set only for an --input pattern)",
}
