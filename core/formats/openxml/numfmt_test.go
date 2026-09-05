package openxml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The expectations are what Excel displays for the same value under the same
// code with an en-US locale; the built-in ids are the ECMA-376 §18.8.30 table.

func TestFormatGeneral(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, "0"},
		{1992, "1992"},
		{100, "100"},
		{0.5, "0.5"},
		{1234.5, "1234.5"},
		{-1234.5, "-1234.5"},
		{0.30000000000000004, "0.3"},
		{1234.5678901234, "1234.56789"},
		{0.1234567890123, "0.123456789"},
		{0.000123456789, "0.000123457"},
		{0.00001, "1E-05"},
		{0.000012345, "1.2345E-05"},
		{12345678901, "12345678901"},
		{123456789012, "1.23457E+11"},
		{1e15, "1E+15"},
		{1.5e11, "1.5E+11"},
		{-0.00001, "-1E-05"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, formatGeneral(c.v), "General of %v", c.v)
	}
}

func TestFormatCellValue(t *testing.T) {
	cases := []struct {
		name string
		code string
		raw  string
		want string
	}{
		// General and its stored-representation noise.
		{"general integer", "General", "1992", "1992"},
		{"general decimal noise", "General", "0.30000000000000004", "0.3"},
		{"general exponent input", "General", "1E+15", "1E+15"},
		{"general with unit", `General" kg"`, "5.5", "5.5 kg"},
		{"general negative", "General", "-5", "-5"},
		{"general colour dropped", "[Blue]General", "5", "5"},
		{"empty code is general", "", "42", "42"},

		// Fixed decimals, rounding half away from zero on fifteen digits.
		{"fixed two", "0.00", "1234.5", "1234.50"},
		{"fixed rounds half up", "0", "2.5", "3"},
		{"fixed rounds half up negative", "0", "-2.5", "-3"},
		{"fixed 1.005", "0.00", "1.005", "1.01"},
		{"fixed 0.05", "0.0", "0.05", "0.1"},
		{"fixed pads", "0.000", "1", "1.000"},
		{"fixed keeps negative zero", "0.00", "-0.001", "-0.00"},
		{"fixed big integer keeps fifteen digits", "0", "123456789012345678", "123456789012346000"},
		{"optional decimals", "0.##", "1.5", "1.5"},
		{"optional decimals keep point", "#.##", "5", "5."},
		{"no forced integer digit", "#.##", "0.5", ".5"},
		{"space placeholder trimmed", "0.??", "1.5", "1.5"},

		// Thousands grouping and scaling.
		{"grouping", "#,##0", "1234567", "1,234,567"},
		{"grouping decimals", "#,##0.00", "1234.5", "1,234.50"},
		{"grouping zero", "#,##0", "0", "0"},
		{"grouping small", "#,##0", "12", "12"},
		{"grouping padded zeros", "0,000", "12", "0,012"},
		{"scale thousands", "#,##0,", "1234567", "1,235"},
		{"scale millions", `#,##0.0,,"M"`, "1234567", "1.2M"},
		{"grouping negative one section", "#,##0.00", "-1234.5", "-1,234.50"},

		// Percent.
		{"percent", "0%", "0.125", "13%"},
		{"percent decimals", "0.00%", "0.125", "12.50%"},
		{"percent whole", "0.0%", "1", "100.0%"},

		// Scientific.
		{"scientific", "0.00E+00", "12345", "1.23E+04"},
		{"scientific small", "0.00E+00", "0.00012345", "1.23E-04"},
		{"scientific zero", "0.00E+00", "0", "0.00E+00"},
		{"engineering", "##0.0E+0", "12345", "12.3E+3"},
		{"scientific minus only", "0.00E-00", "12345", "1.23E04"},
		{"scientific carry", "0.00E+00", "9.999", "1.00E+01"},

		// Fractions.
		{"mixed fraction", "# ?/?", "2.5", "2 1/2"},
		{"mixed fraction whole", "# ?/?", "2", "2"},
		{"mixed fraction below one", "# ?/?", "0.5", "1/2"},
		{"fraction two digits", "# ??/??", "0.333", "1/3"},
		{"fraction fixed denominator", "?/8", "0.75", "6/8"},
		{"improper fraction", "?/?", "2.5", "5/2"},
		{"improper fraction zero", "?/?", "0", "0/1"},
		{"fraction rounds up to the next whole", "# ?/?", "2.96", "3"},

		// Currency and accounting.
		{"currency", `"$"#,##0.00`, "1234.5", "$1,234.50"},
		{"currency negative", `"$"#,##0.00`, "-1234.5", "-$1,234.50"},
		{"currency parentheses", `"$"#,##0.00_);("$"#,##0.00)`, "-1234.5", "($1,234.50)"},
		{"currency padding trimmed", `"$"#,##0.00_);("$"#,##0.00)`, "1234.5", "$1,234.50"},
		{"currency symbol tag", `[$€-407]#,##0.00`, "1234.5", "€1,234.50"},
		{"currency symbol suffix", `#,##0.00 [$kr-414]`, "1234.5", "1,234.50 kr"},
		{"locale tag only", `[$-409]#,##0`, "1234", "1,234"},
		{"escaped dollar", `\$0.00`, "5", "$5.00"},
		{"bare dollar literal", `$#,##0`, "1234", "$1,234"},
		{"quoted unit", `0.00 "USD"`, "5", "5.00 USD"},
		{"accounting positive", builtInNumFmts[44], "1234.5", "$1,234.50"},
		{"accounting negative", builtInNumFmts[44], "-1234.5", "$(1,234.50)"},
		{"accounting zero", builtInNumFmts[44], "0", "$-"},
		{"accounting no symbol zero", builtInNumFmts[41], "0", "-"},
		{"builtin 37 negative", builtInNumFmts[37], "-1234", "(1,234)"},
		{"builtin 7 negative", builtInNumFmts[7], "-1234.5", "($1,234.50)"},

		// Sections.
		{"sections positive", `#,##0.00;(#,##0.00);"-"`, "1234.5", "1,234.50"},
		{"sections negative", `#,##0.00;(#,##0.00);"-"`, "-1234.5", "(1,234.50)"},
		{"sections zero", `#,##0.00;(#,##0.00);"-"`, "0", "-"},
		{"sections rounded to zero stays negative", "#,##0;(#,##0)", "-0.4", "(0)"},
		{"colour negative", "[Red]0", "-5", "-5"},
		{"empty negative section", "0;;", "-5", ""},
		{"empty zero section", "0;-0;", "0", ""},
		{"number under text section", "@", "5", "5"},
		{"condition big", `[>=100]"big";[<0]"neg";"small"`, "150", "big"},
		{"condition negative", `[>=100]"big";[<0]"neg";"small"`, "-5", "neg"},
		{"condition fallback", `[>=100]"big";[<0]"neg";"small"`, "50", "small"},
		{"condition keeps sign", "[>=100]0;0.0", "-5", "-5.0"},

		// Dates (1900 system).
		{"builtin 14", builtInNumFmts[14], "44197", "01-01-21"},
		{"builtin 15", builtInNumFmts[15], "44197", "1-Jan-21"},
		{"builtin 16", builtInNumFmts[16], "44197", "1-Jan"},
		{"builtin 17", builtInNumFmts[17], "44197", "Jan-21"},
		{"builtin 22", builtInNumFmts[22], "44197.5", "1/1/21 12:00"},
		{"iso date", "yyyy-mm-dd", "44197", "2021-01-01"},
		{"iso date floors", "yyyy-mm-dd", "44197.9", "2021-01-01"},
		{"long date", "dddd, mmmm d, yyyy", "44197", "Friday, January 1, 2021"},
		{"month initial", "mmmmm", "44197", "J"},
		{"weekday short", "ddd", "44197", "Fri"},
		{"day month year", "d/m/yyyy", "44197", "1/1/2021"},
		{"upper case letters", "YYYY-MM-DD", "44197", "2021-01-01"},
		{"system long date tag", "[$-F800]dddd, mmmm dd, yyyy", "44197", "Friday, January 01, 2021"},
		{"date with text section", "d-mmm-yy;@", "44197", "1-Jan-21"},
		{"leap bug 59", builtInNumFmts[14], "59", "02-28-00"},
		{"leap bug 60", builtInNumFmts[14], "60", "02-29-00"},
		{"leap bug 61", builtInNumFmts[14], "61", "03-01-00"},
		{"serial one", builtInNumFmts[14], "1", "01-01-00"},
		{"serial zero", builtInNumFmts[14], "0", "01-00-00"},
		{"serial one weekday", "dddd", "1", "Sunday"},
		{"last serial", "yyyy-mm-dd", "2958465", "9999-12-31"},

		// Times.
		{"builtin 18 pm", builtInNumFmts[18], "0.75", "6:00 PM"},
		{"builtin 18 noon", builtInNumFmts[18], "0.5", "12:00 PM"},
		{"builtin 18 midnight", builtInNumFmts[18], "0", "12:00 AM"},
		{"builtin 18 am", builtInNumFmts[18], "0.25", "6:00 AM"},
		{"builtin 19", builtInNumFmts[19], "0.999", "11:58:34 PM"},
		{"builtin 20", builtInNumFmts[20], "0.75", "18:00"},
		{"builtin 21 padded", "hh:mm:ss", "0.5", "12:00:00"},
		{"minutes round", "h:mm", "0.52135416667", "12:31"},
		{"minutes round no carry", "h:mm", "0.5208", "12:30"},
		{"builtin 46 elapsed", builtInNumFmts[46], "1.5", "36:00:00"},
		{"elapsed minutes", "[mm]:ss", "0.001", "01:26"},
		{"builtin 45", builtInNumFmts[45], "0.001", "01:26"},
		{"builtin 47", builtInNumFmts[47], "0.001", "0126.4"},
		{"fractional seconds", "h:mm:ss.000", "0.5000001", "12:00:00.009"},
		{"date time rolls over", "mm/dd/yyyy hh:mm:ss", "44197.999999", "01/02/2021 00:00:00"},
		{"single minute letter", "h:m", "0.75", "18:0"},
		{"minute before seconds", "m:s", "0.001", "1:26"},
		{"month alone", "m", "44197", "1"},
		{"hour with marker", "h AM/PM", "0.75", "6 PM"},
		{"lower case marker", "h:mm am/pm", "0.75", "6:00 pm"},
		{"short marker", "h:mm A/P", "0.75", "6:00 P"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := formatCellValue(c.raw, c.code, false)
			require.True(t, ok, "code %q should be supported", c.code)
			assert.Equal(t, c.want, got, "%s under %q", c.raw, c.code)
		})
	}
}

func TestFormatCellValue_Date1904(t *testing.T) {
	cases := []struct {
		code string
		raw  string
		want string
	}{
		{builtInNumFmts[14], "0", "01-01-04"},
		{builtInNumFmts[14], "44197", "01-02-25"},
		{"dddd", "0", "Friday"},
		{"yyyy-mm-dd", "2957003", "9999-12-31"},
	}
	for _, c := range cases {
		got, ok := formatCellValue(c.raw, c.code, true)
		require.True(t, ok)
		assert.Equal(t, c.want, got, "%s under %q in the 1904 system", c.raw, c.code)
	}
}

// A code the renderer does not handle, or a value the code cannot show, is
// reported and the stored value stands in for the display.
func TestFormatCellValue_UnsupportedFallsBackToRaw(t *testing.T) {
	cases := []struct {
		name string
		code string
		raw  string
	}{
		{"number spelling switch", "[DBNum1]0", "5"},
		{"era year letter", `e"年"`, "44197"},
		{"calendar switch", "B2yyyy", "44197"},
		{"digits inside a date", "yyyy 0", "44197"},
		{"unterminated quote", `"unterminated`, "5"},
		{"dangling escape", `0\`, "5"},
		{"unknown bracket", "[Sheet]0", "5"},
		{"negative serial", "yyyy-mm-dd", "-1"},
		{"serial past the last day", "yyyy-mm-dd", "2958466"},
		{"too many sections", "0;0;0;0;0", "5"},
		{"value is text", "0.00", "abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := formatCellValue(c.raw, c.code, false)
			assert.False(t, ok, "%q should be reported as unsupported", c.code)
			assert.Equal(t, c.raw, got, "the stored value stands in")
		})
	}
}

func TestFormatCellText(t *testing.T) {
	cases := []struct {
		code string
		text string
		want string
	}{
		{"@", "abc", "abc"},
		{`"Note: "@`, "abc", "Note: abc"},
		{"0.00", "abc", "abc"},
		{`0;-0;0;@" (txt)"`, "abc", "abc (txt)"},
		{builtInNumFmts[44], "abc", "abc"},
		{"General", "abc", "abc"},
		{"[DBNum1]0", "abc", "abc"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, formatCellText(c.text, c.code), "%q under %q", c.text, c.code)
	}
}

func TestBuiltInNumFmt(t *testing.T) {
	code, ok := builtInNumFmt(0)
	require.True(t, ok)
	assert.Equal(t, "General", code)
	code, ok = builtInNumFmt(10)
	require.True(t, ok)
	assert.Equal(t, "0.00%", code)
	_, ok = builtInNumFmt(27)
	assert.False(t, ok, "an East Asian calendar id has no invariant code")
	_, ok = builtInNumFmt(164)
	assert.False(t, ok, "custom ids start at 164 and live in the stylesheet")
}
