package filehome

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// Layout tells the file home where a document lives and how to read it. A
// host implements it for a project: the recipe binds each file's format and
// its configuration, and a target template names the file of each edition.
// DirLayout serves a directory with no recipe.
type Layout interface {
	// Locate resolves a document reference. A reference that names no file is
	// an *change.Error with CodeNotFound.
	Locate(ctx context.Context, doc string) (Doc, error)
}

// Doc is a document a layout located.
type Doc struct {
	// Ref is the document's canonical reference: its path relative to the
	// layout's root, slash-separated, or container!entry for an archive
	// member.
	Ref string
	// Path is the file on disk: the document, or the archive that holds it.
	Path string
	// Entry is the archive member, for a document inside an archive.
	Entry string
	// Format reads and writes the document.
	Format Binding
	// SourceLocale is the language the document is written in.
	SourceLocale model.LocaleID
	// Encoding is the document's text encoding; empty is UTF-8.
	Encoding string
	// Editions says how the format holds editions.
	Editions change.Editions
	// TargetLocale is, for a bilingual file, the language of the translation
	// it holds, where a reader has to be told it: a PO catalog's msgstr reads
	// as a translation only when its language is given. Empty leaves the
	// reader to find what the file declares.
	TargetLocale model.LocaleID
	// Edition is set when the reference named the file of one edition of Ref,
	// and is that edition.
	Edition *model.EditionKey
	// EditionFile returns the file of edition k when the edition lives in a
	// file of its own. Nil, or a false answer, leaves the edition with no
	// home unless the format holds it in the document.
	EditionFile func(k model.EditionKey) (EditionFile, bool)
	// NoEditionFile says, for a refusal, why an edition EditionFile names no
	// file for has no home; empty gives the service's general reason.
	NoEditionFile string
	// Derived lists the editions whose own files exist.
	Derived []model.EditionKey
}

// EditionFile is the file of one edition of a document.
type EditionFile struct {
	// Ref is the file as a document reference.
	Ref string
	// Path is the file on disk.
	Path string
	// Format reads and writes the file; the zero Binding is the document's.
	Format Binding
	// Bilingual says the file is a translation catalog in its own right (a
	// PO catalog, an XLIFF or Qt Linguist file) that holds the edition as the
	// translation of each of its units. The home reads and writes it in the
	// edition's language, and a unit with no translation holds no edition.
	// Otherwise the file is a document in the edition's language.
	Bilingual bool
}

// Binding opens a format's reader and writer, each configured for one
// document.
type Binding struct {
	// Name is the format's name.
	Name string
	// NewReader returns a reader configured for the document.
	NewReader func() (format.DataFormatReader, error)
	// NewWriter returns a writer configured for the document, its output
	// encoding included, or an error when the format has no writer.
	NewWriter func() (format.DataFormatWriter, error)
	// Declared, when set, is what a writer outside this process declares it
	// can write (a plugin format's manifest): the change service applies
	// set_attribute, mark and new codes where it declares them, and the
	// plugin's writer spells them when it writes. Nil probes the writer
	// NewWriter returns, which then spells them itself.
	Declared *format.EditCapabilities
}

// Capabilities is what the binding's writer can write beyond what its reader
// read, as the change service applies operations with it: the declaration of
// a writer outside the process (Declared), or what the writer NewWriter
// returns declares, with that writer spelling it. A binding with no writer
// declares nothing.
func (b Binding) Capabilities() change.Capabilities {
	if b.Declared != nil {
		return change.DeclaredCapabilities(b.Name, *b.Declared)
	}
	if b.NewWriter == nil {
		return change.Capabilities{Format: b.Name}
	}
	w, err := b.NewWriter()
	if err != nil || w == nil {
		return change.Capabilities{Format: b.Name}
	}
	return change.WriterCapabilities(b.Name, w)
}

// PluginDeclared returns what reg records a plugin format's writer declares,
// for Binding.Declared, or nil for a built-in format, whose writer is probed
// in process.
func PluginDeclared(reg *registry.FormatRegistry, name string) *format.EditCapabilities {
	if reg == nil {
		return nil
	}
	info := reg.FormatInfo(registry.FormatID(name))
	if info == nil || info.Source == "" || info.Source == registry.SourceBuiltIn {
		return nil
	}
	declared := info.EditCapabilities.Clone()
	return &declared
}

// RegistryBinding binds format name to its reader and writer in reg, with the
// defaults' configuration, writing in encoding (empty is the writer's
// default).
func RegistryBinding(reg *registry.FormatRegistry, name, encoding string) Binding {
	id := registry.FormatID(name)
	return Binding{
		Name:      name,
		Declared:  PluginDeclared(reg, name),
		NewReader: func() (format.DataFormatReader, error) { return reg.NewReader(id) },
		NewWriter: func() (format.DataFormatWriter, error) {
			w, err := reg.NewWriter(id)
			if err != nil {
				return nil, err
			}
			if encoding != "" {
				w.SetEncoding(encoding)
			}
			return w, nil
		},
	}
}

// DirLayout serves the documents under one directory, outside any project:
// each format detected from the file, read and written with its defaults. A
// bilingual format holds its editions in the document; a monolingual
// document holds one edition, and another has no home.
type DirLayout struct {
	// Root is the directory references are relative to. A reference may not
	// leave it.
	Root string
	// Formats detects formats and opens their readers and writers.
	Formats *registry.FormatRegistry
	// SourceLocale is the language documents are written in.
	SourceLocale model.LocaleID
	// Format, when set, names the format of every document instead of
	// detecting it.
	Format string
	// Encoding is the encoding documents are read and written in; empty is
	// UTF-8.
	Encoding string
	// TargetLocale is the language of the translation a bilingual document
	// holds (Doc.TargetLocale).
	TargetLocale model.LocaleID
}

// Locate resolves doc under the root.
func (l DirLayout) Locate(_ context.Context, doc string) (Doc, error) {
	ref, path, entry, err := ResolvePath(l.Root, doc)
	if err != nil {
		return Doc{}, err
	}
	name := l.Format
	if name == "" {
		name, err = DetectFormat(l.Formats, path, entry)
		if err != nil {
			return Doc{}, err
		}
	}
	d := Doc{Ref: ref, Path: path, Entry: entry, Format: RegistryBinding(l.Formats, name, l.Encoding),
		SourceLocale: l.SourceLocale, Encoding: l.Encoding, Editions: change.EditionsPerFile}
	if info := l.Formats.FormatInfo(registry.FormatID(name)); info != nil && info.Interchange {
		d.Editions = change.EditionsInFile
		d.TargetLocale = l.TargetLocale
	}
	return d, nil
}

// ResolvePath resolves a document reference under root: a relative path, or
// container!entry for a member of an archive under root. It returns the
// canonical reference, the file on disk and the archive member. A path that
// leaves root, or names no file, is refused.
func ResolvePath(root, doc string) (ref, path, entry string, err error) {
	if doc == "" {
		return "", "", "", &change.Error{Code: change.CodeInvalid, Field: "at/doc", Message: "the reference names no document"}
	}
	rel, entry := splitEntry(root, doc)
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) {
		r, rerr := filepath.Rel(root, clean)
		if rerr != nil {
			return "", "", "", outside(doc)
		}
		clean = r
	}
	if !filepath.IsLocal(clean) {
		return "", "", "", outside(doc)
	}
	path = filepath.Join(root, clean)
	info, serr := os.Stat(path)
	switch {
	case serr != nil:
		return "", "", "", &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + doc}
	case info.IsDir():
		return "", "", "", &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: doc + " is a directory, not a document"}
	}
	ref = filepath.ToSlash(clean)
	if entry != "" {
		ref += "!" + entry
	}
	return ref, path, entry, nil
}

func outside(doc string) error {
	return &change.Error{Code: change.CodeInvalid, Field: "at/doc", Message: doc + " is outside the documents this service edits"}
}

// splitEntry splits container!entry where the part before the bang is an
// archive under root; any other reference is a plain path.
func splitEntry(root, doc string) (string, string) {
	for i := range len(doc) {
		if doc[i] != '!' {
			continue
		}
		left, right := doc[:i], doc[i+1:]
		if right == "" || !container.IsContainerPath(left) {
			continue
		}
		p := left
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, filepath.FromSlash(left))
		}
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			continue
		}
		return left, strings.TrimPrefix(right, "/")
	}
	return doc, ""
}

// DetectFormat detects the format of the file at path, or of the archive
// member entry inside it.
func DetectFormat(reg *registry.FormatRegistry, path, entry string) (string, error) {
	if entry == "" {
		id, err := reg.Detect(path, registry.DetectOptions{})
		if err != nil || id == "" {
			return "", &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + filepath.Base(path)}
		}
		return string(id), nil
	}
	id, err := reg.Detect(entry, registry.DetectOptions{Content: func() (io.ReadSeeker, error) {
		data, _, oerr := container.OpenEntry(path, entry)
		if oerr != nil {
			return nil, oerr
		}
		return strings.NewReader(string(data)), nil
	}})
	if err != nil || id == "" {
		return "", &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + entry}
	}
	return string(id), nil
}

// Formats answers what the service knows about a format from reg.
type Formats struct{ Registry *registry.FormatRegistry }

// Facts reads a format's registration.
func (f Formats) Facts(name string) (change.FormatFacts, bool) {
	if f.Registry == nil {
		return change.FormatFacts{}, false
	}
	info := f.Registry.FormatInfo(registry.FormatID(name))
	if info == nil {
		return change.FormatFacts{}, false
	}
	return change.FormatFacts{
		Name:              string(info.Name),
		Editable:          info.HasReader && info.HasWriter,
		Interchange:       info.Interchange,
		RoundTrip:         info.RoundTrip,
		InlineAnnotations: info.InlineAnnotations,
		Edit:              info.EditCapabilities,
	}, true
}
