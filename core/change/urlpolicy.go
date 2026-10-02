package change

import (
	"fmt"
	"html"
	"strings"
)

// Attribute values that run code where a document is read. A value an
// operation writes to an attribute (set_attribute, mark, a new code in a runs
// payload) is refused as invalid when a reader of the document would run it:
//
//   - an event-handler attribute (onclick, onload, any on* name) holds script,
//     and an iframe's srcdoc holds a document, whatever the value, so neither
//     is written;
//   - a URL-valued attribute (a link's href, an image's src, a form's action,
//     an object's data, an SVG animation's to) is refused a javascript: or
//     vbscript: URL, or a data: URL holding anything but a raster image.
//
// The check runs before any format spells the value, so it holds for every
// format, a plugin's included. A value the document already holds is read,
// kept and written as it is.

// attrScript reports whether attribute name holds script or a document that
// runs where the document is read: an event handler (on followed by letters,
// in any namespace and case) or an iframe's srcdoc.
func attrScript(name string) bool {
	local := strings.ToLower(localAttrName(name))
	if local == "srcdoc" {
		return true
	}
	if len(local) <= 2 || !strings.HasPrefix(local, "on") {
		return false
	}
	for _, c := range local[2:] {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// urlAttrs maps the local name of an attribute whose value a reader of the
// document follows, loads or submits to as a URL to how the value holds its
// URLs. An attribute is matched by its local name in any namespace and case,
// so xlink:href is an href.
var urlAttrs = map[string]urlList{
	"href": oneURL, "src": oneURL, "action": oneURL, "formaction": oneURL,
	"data": oneURL, "cite": oneURL, "poster": oneURL, "background": oneURL,
	"codebase": oneURL, "classid": oneURL, "longdesc": oneURL, "lowsrc": oneURL,
	"dynsrc": oneURL, "usemap": oneURL, "manifest": oneURL, "icon": oneURL,
	"profile": oneURL,
	// SVG animation elements (set, animate) write to, from and by, or each
	// of values, into the attribute they animate, which may be an href.
	"to": oneURL, "from": oneURL, "by": oneURL, "values": semicolonURLs,
	// Candidate lists: image candidates with their descriptors, URLs pinged
	// on a click, archives an object loads.
	"srcset": spacedURLs, "imagesrcset": spacedURLs, "ping": spacedURLs, "archive": spacedURLs,
}

// urlList is how an attribute value holds its URLs.
type urlList int

const (
	oneURL        urlList = iota // the value is one URL
	spacedURLs                   // URLs and descriptors separated by commas or white space
	semicolonURLs                // values separated by semicolons
)

// urls returns the URLs value holds.
func (l urlList) urls(value string) []string {
	switch l {
	case spacedURLs:
		return strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
		})
	case semicolonURLs:
		return strings.Split(value, ";")
	}
	return []string{value}
}

// localAttrName is name without its namespace prefix.
func localAttrName(name string) string {
	if _, local, ok := strings.CutLast(name, ":"); ok {
		return local
	}
	return name
}

// urlReadings returns the readings a reader of the document may make of
// value: the value as given, and the value with HTML and XML character
// references decoded (&#106;, &colon;) or CommonMark backslash escapes
// resolved (\:), in every order and repeated. Formats differ in which they
// decode, and a renderer decodes what a format's reader kept: a Markdown
// destination's references and escapes reach the HTML a renderer produces,
// where the browser decodes the references. Judging every reading holds for
// every format and renderer; no URL a document links to in earnest needs a
// reference or an escape to spell its scheme.
func urlReadings(value string) []string {
	const maxReadings = 64
	seen := map[string]bool{value: true}
	out := []string{value}
	for frontier := []string{value}; len(frontier) > 0 && len(out) < maxReadings; {
		var next []string
		for _, v := range frontier {
			for _, d := range [...]string{html.UnescapeString(v), resolveBackslashEscapes(v)} {
				if !seen[d] && len(out) < maxReadings {
					seen[d] = true
					out = append(out, d)
					next = append(next, d)
				}
			}
		}
		frontier = next
	}
	return out
}

// resolveBackslashEscapes resolves CommonMark backslash escapes: a backslash
// before an ASCII punctuation character stands for that character.
func resolveBackslashEscapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && asciiPunct(s[i+1]) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func asciiPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// unsafeURL is a URL that runs code where the document is read.
type unsafeURL struct {
	// scheme is javascript, vbscript or data.
	scheme string
	// media is a data: URL's media type.
	media string
}

func (u unsafeURL) String() string {
	if u.scheme == "data" {
		media := u.media
		if media == "" {
			media = "no media type"
		}
		return "data: URL holding " + media
	}
	return u.scheme + ": URL"
}

// unsafeURLScheme returns how url runs code where the document is read: a
// javascript: or vbscript: URL, or a data: URL holding anything but a raster
// image (an SVG image, or any XML, can carry script). The URL is read as a
// browser reads it: leading and trailing control characters and spaces are
// skipped, a tab or a line break inside it is ignored, and the scheme's case
// does not matter.
func unsafeURLScheme(url string) (unsafeURL, bool) {
	url = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, strings.TrimFunc(url, func(r rune) bool { return r <= ' ' }))
	colon := strings.IndexByte(url, ':')
	if colon <= 0 {
		return unsafeURL{}, false
	}
	scheme := strings.ToLower(url[:colon])
	for i, c := range scheme {
		letter := c >= 'a' && c <= 'z'
		if !letter && (i == 0 || !(c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return unsafeURL{}, false
		}
	}
	switch scheme {
	case "javascript", "vbscript":
		return unsafeURL{scheme: scheme}, true
	case "data":
		media := strings.ToLower(url[colon+1:])
		if end := strings.IndexAny(media, ";,"); end >= 0 {
			media = media[:end]
		}
		media = strings.TrimSpace(media)
		raster := strings.HasPrefix(media, "image/") && !strings.Contains(media, "svg") && !strings.Contains(media, "xml")
		return unsafeURL{scheme: scheme, media: media}, !raster
	}
	return unsafeURL{}, false
}

// attrRunsCode returns, as an invalid error with the reason, why value for
// attribute name would run code where the document is read; others are the
// code's attributes with the value in place. An SVG animation that names an
// href in attributeName writes its to, from, by or values there, so setting
// attributeName checks them. nameField and valueField are the fields of the
// request the error names.
func attrRunsCode(name, value string, others map[string]string, nameField, valueField string) *Error {
	if attrScript(name) {
		return &Error{Code: CodeInvalid, Field: nameField,
			Message: fmt.Sprintf("the %s attribute holds script or a document that runs where the document is read, so no operation writes it", name)}
	}
	if list, ok := urlAttrs[strings.ToLower(localAttrName(name))]; ok {
		if err := checkURLValue(name, value, list, valueField); err != nil {
			return err
		}
	}
	if strings.EqualFold(localAttrName(name), "attributeName") && strings.EqualFold(localAttrName(strings.TrimSpace(value)), "href") {
		for _, other := range sortedKeys(others) {
			if list, ok := urlAttrs[strings.ToLower(localAttrName(other))]; ok && other != name {
				if err := checkURLValue(other, others[other], list, valueField); err != nil {
					err.Message = fmt.Sprintf("an animation of an href writes its %s attribute there: %s", other, err.Message)
					return err
				}
			}
		}
	}
	return nil
}

// checkURLValue refuses a value of a URL-valued attribute holding a URL that,
// in any reading of it (urlReadings), runs code where the document is read.
func checkURLValue(name, value string, list urlList, field string) *Error {
	for _, reading := range urlReadings(value) {
		for _, url := range list.urls(reading) {
			if bad, unsafe := unsafeURLScheme(url); unsafe {
				return &Error{Code: CodeInvalid, Field: field,
					Message: fmt.Sprintf("the %s attribute cannot be set to a %s, which runs code where the document is read (character references and backslash escapes are read as a reader of the document decodes them); give an http, https, mailto or relative URL, or a raster image as a data: URL", name, bad)}
			}
		}
	}
	return nil
}
