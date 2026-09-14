package comments_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/plugins/sourcecode/internal/comments"
)

// TestCLexicalScan holds the C and C++ lexical scan to the comments a compiler
// reads, one construct at a time. libclang reads the same comments in each
// source.
func TestCLexicalScan(t *testing.T) {
	for _, tc := range []struct {
		name, language, src string
		want                []string
	}{
		{"a raw string holding a comment opener and a false end", "cpp", "auto raw = R\"x(a)\" /* not a comment */ \")x\"; // after\n", []string{"// after"}},
		{"a raw string under each prefix", "cpp", "auto a = u8R\"(/*)\"; auto b = LR\"(/*)\"; auto c = uR\"(/*)\"; auto d = UR\"(/*)\"; // after\n", []string{"// after"}},
		{"a raw string keeps a splice as written", "cpp", "auto raw = R\"(a\\\n/*)\"; // after\n", []string{"// after"}},
		{"an identifier ending in R before a string", "cpp", "auto s = FOOR\"(\" /* a comment */ \")\";\n", []string{"/* a comment */"}},
		{"a line splice extends a line comment", "c", "// continued \\\nonto this line\nint x;\n", []string{"// continued \\\nonto this line"}},
		{"a splice with spaces before its line break", "c", "// continued \\  \nonto this line\nint x;\n", []string{"// continued \\  \nonto this line"}},
		{"a splice before a CRLF", "c", "// continued \\\r\nonto this line\r\nint x;\r\n", []string{"// continued \\\r\nonto this line"}},
		{"a splice inside a comment's opener", "c", "/\\\n* spliced */ int x;\n", []string{"/\\\n* spliced */"}},
		{"a splice inside a comment's closer", "c", "/* spliced *\\\n/ int x;\n", []string{"/* spliced *\\\n/"}},
		{"a splice inside a string", "c", "char *s = \"a\\\n// b\"; // after\n", []string{"// after"}},
		{"a character literal '/' next to an asterisk", "c", "int n = '/'*2; /* after */\n", []string{"/* after */"}},
		{"a character literal holding a double quote", "c", "char q = '\"'; // after\n", []string{"// after"}},
		{"an escaped quote in a character literal", "c", "char q = '\\''; // after\n", []string{"// after"}},
		{"prefixed character and string literals", "cpp", "auto a = u8'/'; auto b = L\"/*\"; auto c = U'*'; auto d = u\"//\"; // after\n", []string{"// after"}},
		{"a digit separator opens no character literal", "cpp", "int n = 1'000; // after\n", []string{"// after"}},
		{"digit separators in a hexadecimal and a floating number", "c", "int n = 0x7f'ff; double d = 1'0.5e+1'0; // after\n", []string{"// after"}},
		{"a string holding both markers", "c", "char *s = \"// not a comment /* either */\"; // after\n", []string{"// after"}},
		{"an escaped quote in a string", "c", "char *s = \"a\\\"// b\"; // after\n", []string{"// after"}},
		{"an unterminated character literal runs to the end of its line", "c", "#if 0\nIt's /* inside the literal */ prose\n#endif\n// after\n", []string{"// after"}},
		{"a comment in a skipped branch", "c", "#if 0\n// in a skipped branch\n#endif\n", []string{"// in a skipped branch"}},
		{"a comment after a macro's value", "c", "#define LIMIT 10 // after\n", []string{"// after"}},
		{"a header name holds comment markers", "c", "#include <sys//types.h> // after\n#  include_next </*x*/> /* after */\n", []string{"// after", "/* after */"}},
		{"a digraph opens an include", "c", "%:include <a//b.h> // after\n", []string{"// after"}},
		{"a comment between a directive's parts", "c", "#include /* between */ <a//b.h>\n", []string{"/* between */"}},
		{"__has_include takes a header name", "cpp", "#if __has_include(<a//b.h>) // after\n#endif\n", []string{"// after"}},
		{"a header name the line ends before closing", "c", "#include <unclosed // after\n", []string{"// after"}},
		{"a less-than sign outside a directive", "c", "int b = a <c; // after >\n", []string{"// after >"}},
		{"a hash after code opens no directive", "c", "x # include <a//b> \n", []string{"//b> "}},
		{"a lone carriage return ends a line comment", "c", "// a\rint x; // b\n", []string{"// a", "// b"}},
		{"a raw string prefix C reads one way", "c", "char *s = R\"(a)\"; // after\n", []string{"// after"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := comments.LexicalComments(tc.language, []byte(tc.src))
			require.NoError(t, err)
			var got []string
			for _, s := range spans {
				got = append(got, tc.src[s[0]:s[1]])
			}
			assert.Equal(t, tc.want, got)
		})
	}

	for _, tc := range []struct {
		name, language, src, reason string
	}{
		{"a block comment never closed", "c", "int x; /* never closed\n", "a block comment is never closed"},
		{"a raw string never closed", "cpp", "auto s = R\"x(never closed)\";\n", "a raw string is never closed"},
		{"a raw string delimiter holding a space", "cpp", "auto s = R\"a b(x)a b\";\n", "a raw string's delimiter is malformed"},
		{"a raw string delimiter longer than sixteen characters", "cpp", "auto s = R\"abcdefghijklmnopq(x)abcdefghijklmnopq\";\n", "a raw string's delimiter is malformed"},
		{"a raw string prefix C reads two ways", "c", "char *s = R\"x(a\"b)x\"; // after\n", "the comments read differently with and without raw string literals"},
		{"a trigraph read two ways", "c", "// a ??/\nint x; // b\n", "the comments read differently with and without trigraphs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := comments.LexicalComments(tc.language, []byte(tc.src))
			require.Error(t, err)
			assert.Equal(t, tc.reason, err.Error())
		})
	}
}
