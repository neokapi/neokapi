# Bowrain Implementation Plan

Complete implementation plan for the Bowrain Localization Workbench desktop application covering translation project management and a visual translation editor.

---

## 1. Translation Project System

### 1.1 Package Format (.kaz)

A `.kaz` (Kapi Archive Zip) file is a standard ZIP archive containing a self-contained translation project. The format is designed to be portable, version-aware, and tooling-independent.

**Structure:**

```
myproject.kaz
├── manifest.yaml              # Package metadata and file inventory
├── files/                     # Original source files
│   ├── index.html
│   ├── messages.json
│   └── strings.properties
├── xliff/                     # Extracted XLIFF work files (per locale pair)
│   ├── index.html.xlf
│   ├── messages.json.xlf
│   └── strings.properties.xlf
├── assets/                    # Referenced media/binary assets
│   └── logo.png
└── tm/                        # Project-level translation memory
    └── project.tmx
```

**manifest.yaml schema:**

```yaml
name: "My Translation Project"
version: "1.0"
gokapi_version: "0.1.0"
source_locale: "en"
target_locales:
  - "fr"
  - "de"
created_at: "2026-01-30T12:00:00Z"
modified_at: "2026-01-30T12:00:00Z"
formats_required:
  - "html"
  - "json"
  - "properties"
plugins_required: []
files:
  - path: "index.html"
    format: "html"
    size: 4096
    block_count: 12
    word_count: 245
  - path: "messages.json"
    format: "json"
    size: 1024
    block_count: 8
    word_count: 62
```

### 1.2 Project Lifecycle

```
Create Project → Add Files → Extract Blocks → Translate → Export → Save/Package
       ↕              ↕            ↕              ↕          ↕
  Open .kaz      Drag & Drop   Auto on add    Editor UI   Per-file
```

### 1.3 Backend API (Go)

**Project Management Methods (bound to Wails frontend):**

| Method | Signature | Description |
|--------|-----------|-------------|
| `CreateProject` | `(name, sourceLang string, targetLangs []string) (*ProjectInfo, error)` | Create new in-memory project |
| `OpenProject` | `(path string) (*ProjectInfo, error)` | Open .kaz file or directory |
| `SaveProject` | `(projectID string) error` | Save project to its current path |
| `SaveProjectAs` | `(projectID, path string) error` | Save project as .kaz to new path |
| `CloseProject` | `(projectID string) error` | Close and release project resources |
| `GetProject` | `(projectID string) (*ProjectInfo, error)` | Get current project info |
| `AddFiles` | `(projectID string, filePaths []string) (*ProjectInfo, error)` | Import files, auto-detect format, extract blocks |
| `RemoveFile` | `(projectID, fileName string) (*ProjectInfo, error)` | Remove file from project |
| `ListProjectFiles` | `(projectID string) ([]ProjectFile, error)` | List files in project |

**Translation Editor Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `GetFileBlocks` | `(projectID, fileName string) ([]BlockInfo, error)` | Get all blocks for a file |
| `UpdateBlockTarget` | `(req UpdateBlockRequest) error` | Update target text for a block |
| `PseudoTranslateFile` | `(projectID, fileName, targetLocale string) (*TranslationStats, error)` | Pseudo-translate all blocks |
| `AITranslateFile` | `(req AITranslateFileRequest) (*TranslationStats, error)` | AI-translate all blocks |
| `TMTranslateFile` | `(projectID, fileName, targetLocale string) (*TranslationStats, error)` | Leverage TM for all blocks |
| `GetWordCount` | `(projectID, fileName string) (*WordCountResult, error)` | Get word/char counts |
| `ExportTranslatedFile` | `(projectID, fileName, targetLocale string) (string, error)` | Write translated file to disk |
| `OpenFileInOS` | `(filePath string) error` | Open file in OS default application |

**Data Types:**

```go
type ProjectInfo struct {
    ID            string        `json:"id"`
    Name          string        `json:"name"`
    SourceLocale  string        `json:"source_locale"`
    TargetLocales []string      `json:"target_locales"`
    Path          string        `json:"path"`
    Files         []ProjectFile `json:"files"`
    CreatedAt     string        `json:"created_at"`
    ModifiedAt    string        `json:"modified_at"`
}

type ProjectFile struct {
    Name       string `json:"name"`
    Format     string `json:"format"`
    Size       int64  `json:"size"`
    BlockCount int    `json:"block_count"`
    WordCount  int    `json:"word_count"`
}

type BlockInfo struct {
    ID           string            `json:"id"`
    Source       string            `json:"source"`
    Targets      map[string]string `json:"targets"`
    Translatable bool              `json:"translatable"`
    HasSpans     bool              `json:"has_spans"`
    Properties   map[string]string `json:"properties"`
}

type TranslationStats struct {
    TotalBlocks      int `json:"total_blocks"`
    TranslatedBlocks int `json:"translated_blocks"`
    WordCount        int `json:"word_count"`
}

type WordCountResult struct {
    SourceWords int            `json:"source_words"`
    SourceChars int            `json:"source_chars"`
    TargetWords map[string]int `json:"target_words"`
    TargetChars map[string]int `json:"target_chars"`
}
```

---

## 2. Visual Translation Editor

### 2.1 Editor Layout

```
┌──────────────────────────────────────────────────────────┐
│  [< Back to Project]    file.html    [Word Count] [Export]│
│  ─────────────────────────────────────────────────────── │
│  Toolbar: [Pseudo] [AI Translate] [TM Lookup] [Settings] │
│  ─────────────────────────────────────────────────────── │
│  Progress: ████████░░ 80% (40/50 translated)  Lang: [fr] │
│  ═══════════════════════════════════════════════════════ │
│  #  │ Source                  │ Target                   │
│  ── │ ─────────────────────── │ ──────────────────────── │
│  1  │ Welcome to our site     │ Bienvenue sur notre site │
│  2  │ Click here to begin     │ Cliquez ici pour...      │
│ >3  │ Contact us              │ [editable input]         │
│  4  │ About our company       │                          │
│  5  │ Privacy Policy          │                          │
│  ─────────────────────────────────────────────────────── │
│  Status: Block 3 of 50 │ Source words: 245 │ Unsaved    │
└──────────────────────────────────────────────────────────┘
```

### 2.2 Editor Features

- **Block Navigation**: Arrow keys or click to navigate between translation units
- **Inline Editing**: Click on target cell to edit, auto-save on blur
- **Progress Tracking**: Visual progress bar showing translation completion
- **Locale Selector**: Switch between target locales
- **Search/Filter**: Search blocks by source or target text
- **Status Indicators**: Visual cues for translated, untranslated, fuzzy-matched blocks
- **Keyboard Shortcuts**: Enter to confirm and advance, Escape to cancel, Ctrl+S to save

### 2.3 Translation Tools (Toolbar)

| Tool | Action | Description |
|------|--------|-------------|
| Pseudo-Translate | One-click | Generate pseudo-translations for all untranslated blocks |
| AI Translate | One-click | Translate using configured AI provider |
| TM Lookup | One-click | Leverage translation memory matches |
| Word Count | Display | Show source/target word and character counts |
| Export | One-click | Export translated file and open in OS |

### 2.4 Document Preview

The editor includes a rendered HTML preview of the document alongside the block grid. Source documents are converted to an HTML representation that highlights the currently selected block, providing visual context for the translator.

---

## 3. Frontend Architecture

### 3.1 Component Tree

```
App
├── Sidebar (updated with Projects view)
├── Header
└── Main Content
    ├── ProjectDashboard (list/create/open projects)
    │   ├── ProjectCard (per project)
    │   └── CreateProjectDialog
    ├── ProjectView (single project workspace)
    │   ├── FileDropZone (drag & drop)
    │   ├── ProjectFileList (file inventory)
    │   └── ProjectToolbar (save, export, settings)
    ├── TranslationEditor (per-file editor)
    │   ├── EditorToolbar (pseudo, AI, TM, export)
    │   ├── ProgressBar
    │   ├── BlockGrid (source | target columns)
    │   │   └── BlockRow (per translation unit)
    │   └── StatusBar
    ├── FormatList (existing)
    ├── ToolList (existing)
    ├── FlowList (existing)
    ├── ConvertPanel (existing)
    └── TranslatePanel (existing)
```

### 3.2 State Management

Project state is managed in the App component and passed down via props:
- `activeProject: ProjectInfo | null` — currently open project
- `activeFile: string | null` — file being edited
- `blocks: BlockInfo[]` — blocks for the active file
- `selectedBlockIndex: number` — currently selected block

### 3.3 New Views

| View ID | Component | Description |
|---------|-----------|-------------|
| `projects` | `ProjectDashboard` | Project list with create/open |
| `project` | `ProjectView` | Single project file management |
| `editor` | `TranslationEditor` | Visual block-by-block editor |

---

## 4. Implementation Phases

### Phase A: Go Backend — Project Management

1. Define project types in `apps/bowrain/backend/project.go`
2. Implement .kaz package read/write in `apps/bowrain/backend/kaz.go`
3. Add project CRUD methods to App struct
4. Add file import with auto-detection and block extraction
5. Write unit tests

### Phase B: Go Backend — Translation Editor

1. Add block extraction and serialization
2. Add block update (target text editing)
3. Integrate pseudo-translate, word count tools
4. Integrate AI translation (with provider config)
5. Integrate TM leverage
6. Add file export (write translated file)
7. Add OpenFileInOS using `open`/`xdg-open`
8. Write unit tests

### Phase C: Frontend — Project Management

1. Update Sidebar with Projects view
2. Build ProjectDashboard component
3. Build CreateProjectDialog component
4. Build ProjectView with file list
5. Build FileDropZone using Wails OnFileDrop API
6. Wire up to backend via Wails bindings

### Phase D: Frontend — Translation Editor

1. Build TranslationEditor component
2. Build EditorToolbar with tool buttons
3. Build BlockGrid with source/target columns
4. Build BlockRow with inline editing
5. Add keyboard navigation (arrow keys, Enter, Escape)
6. Add progress bar and status bar
7. Add locale selector
8. Wire up pseudo-translate, AI translate, TM, export
9. Add search/filter functionality

### Phase E: Playwright E2E Tests

1. Set up Playwright with Vite dev server
2. Mock Wails backend bindings
3. Write tests for project creation flow
4. Write tests for file list and management
5. Write tests for editor navigation
6. Write tests for translation workflows
7. Write tests for export flow

---

## 5. Test Plan

### 5.1 Go Unit Tests

**Project Management (`backend/project_test.go`):**

| Test | Description |
|------|-------------|
| `TestCreateProject` | Create project with name, source locale, target locales |
| `TestCreateProject_Validation` | Reject empty name, invalid locales |
| `TestAddFiles` | Add files, verify format detection and block extraction |
| `TestAddFiles_UnsupportedFormat` | Handle unsupported file formats gracefully |
| `TestRemoveFile` | Remove file from project |
| `TestGetFileBlocks` | Get blocks for a specific file |
| `TestUpdateBlockTarget` | Update target text for a block |
| `TestCloseProject` | Close project releases resources |

**Package Format (`backend/kaz_test.go`):**

| Test | Description |
|------|-------------|
| `TestSaveAndOpenKaz` | Round-trip: create project, add files, save .kaz, reopen, verify |
| `TestKazManifest` | Verify manifest.yaml content |
| `TestKazWithMultipleFiles` | Package with HTML, JSON, properties files |
| `TestKazWithTranslations` | Package preserves translated blocks |
| `TestOpenInvalidKaz` | Gracefully handle corrupt/invalid .kaz files |

**Translation Tools (`backend/editor_test.go`):**

| Test | Description |
|------|-------------|
| `TestPseudoTranslateFile` | Pseudo-translate all blocks in a file |
| `TestGetWordCount` | Word count for source and target |
| `TestExportTranslatedFile` | Export translated file to disk |
| `TestAITranslateFile_Mock` | AI translate with mock provider |
| `TestTMTranslateFile` | TM leverage with pre-loaded entries |

### 5.2 Playwright E2E Tests

**Project Dashboard (`e2e/project-dashboard.spec.ts`):**

| Test | Description |
|------|-------------|
| `should display empty state on first load` | No projects message shown |
| `should create a new project` | Fill form, submit, verify project appears |
| `should navigate to project view` | Click project card, verify file list |

**Project View (`e2e/project-view.spec.ts`):**

| Test | Description |
|------|-------------|
| `should display project files` | Files listed with format and word count |
| `should open file in editor` | Click file, verify editor opens |
| `should show file drop zone` | Drop zone visible and labeled |

**Translation Editor (`e2e/translation-editor.spec.ts`):**

| Test | Description |
|------|-------------|
| `should display blocks with source text` | Blocks rendered in grid |
| `should navigate blocks with keyboard` | Arrow keys move selection |
| `should edit target text inline` | Click target cell, type, blur to save |
| `should show progress bar` | Progress updates as blocks are translated |
| `should pseudo-translate all blocks` | Click pseudo button, verify targets filled |
| `should show word count` | Word count displayed in status bar |
| `should switch target locale` | Locale selector changes displayed targets |
| `should export translated file` | Click export, verify success message |

### 5.3 Validation Checklist

**Project Management:**

- [ ] Create a new project with name and language settings
- [ ] Add files via drag and drop
- [ ] Add files via file picker
- [ ] File format is auto-detected
- [ ] Blocks are extracted from added files
- [ ] File list shows format, block count, word count
- [ ] Remove a file from the project
- [ ] Save project as .kaz package
- [ ] Open a .kaz package
- [ ] Opened .kaz preserves all translations
- [ ] Project handles 50+ files without issues

**Visual Translation Editor:**

- [ ] Editor displays all blocks for selected file
- [ ] Source text is read-only
- [ ] Target text is editable inline
- [ ] Arrow keys navigate between blocks
- [ ] Enter confirms edit and moves to next block
- [ ] Escape cancels current edit
- [ ] Progress bar shows completion percentage
- [ ] Locale selector switches target language
- [ ] Search/filter finds blocks by text
- [ ] Translated blocks have visual indicator
- [ ] Untranslated blocks are clearly marked

**Translation Tools:**

- [ ] One-click pseudo-translate fills all targets
- [ ] One-click AI translate calls provider and fills targets
- [ ] TM leverage finds and applies matches
- [ ] Word count shows source and target counts
- [ ] Export writes translated file to disk
- [ ] One-click opens exported file in OS default app

**Package Format (.kaz):**

- [ ] .kaz file is a valid ZIP archive
- [ ] manifest.yaml contains correct metadata
- [ ] Source files are preserved in files/ directory
- [ ] XLIFF work files are in xliff/ directory
- [ ] Translations round-trip through save/open
- [ ] Package includes project TM if available

---

## 6. Dependencies

### Frontend (new)
- `@playwright/test` — E2E testing framework

### Backend (existing, no new dependencies)
- All required packages already in go.mod
- archive/zip (stdlib) for .kaz packaging
- gopkg.in/yaml.v3 for manifest serialization
- os/exec for OpenFileInOS
