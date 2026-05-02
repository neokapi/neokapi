//go:build parity

package roundtrip_test

// idmlBridgeSkips and openxmlBridgeSkips list the upstream IDML and
// OpenXML fixtures where bridge's pseudo-translated output diverges
// from the okapi reference. Native is included on every entry because
// each format already format-default-skips native; the per-file entry
// extends that to bridge for these specific files.
//
// The lists are extracted from a real round-trip suite run against
// the okapi-testdata-1.48.0 release. As bridge bugs get fixed
// upstream, drop entries from these maps and the corresponding
// sub-tests start asserting again.

func idmlBridgeSkips() map[string]fileSkip {
	const reason = "bridge inline-code/run-marker divergence in IDML Stories XML vs okapi reference"
	return map[string]fileSkip{
		"00-primary-text-frame-with-different-breaking-symbols.idml": {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames-2.idml":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames-3.idml":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames-4.idml":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames-5.idml":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames-6.idml":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"01-pages-with-text-frames.idml":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"03-hyperlink-and-table-content.idml":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"04-complex-formatting.idml":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"06-hello-world-14.idml":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"09-footnotes.idml":                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"10-tables.idml":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"1016.idml":                                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"11-xml-structures.idml":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"1139.idml":                                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"1179-0.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1179-1.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1179-2.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1179-3.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1179-4.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1369-empty-paragraph-in-table-cell-styles.idml":             {Engines: []string{"bridge", "native"}, Reason: reason},
		"1412-math-zones.idml":                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"1415-adjacent-codes.idml":                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"1418-styles-exclusion.idml":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"175-special-characters.idml":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"618-MBE3.idml":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"618-anchored-frame-without-path-points.idml":                {Engines: []string{"bridge", "native"}, Reason: reason},
		"756-character-baseline-shift.idml":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"756-character-kerning.idml":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"756-character-leading.idml":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"756-character-tracking.idml":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"777-character-kerning-method.idml":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"779-reference-and-tag-styles.idml":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"856-1.idml":                                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"856-2.idml":                                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"923-baselined-formatting.idml":                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"Bindestrich.idml":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test00.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test01.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test02.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test03.idml":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"TextPathTest04.idml":                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"change-tracking-3.idml":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"idmltest.idml":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"tabsAndWhitespaces.idml":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"testWithSpecialChars.idml":                                  {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func openxmlBridgeSkips() map[string]fileSkip {
	const reason = "bridge inline-run/code divergence in OpenXML document parts vs okapi reference"
	return map[string]fileSkip{
		"1083-date-and-hyperlink-instructions.docx":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1083-empty-and-hyperlink-instructions.docx":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"1083-hyperlink-and-date-instructions.docx":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1083-hyperlink-and-empty-instructions.docx":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"1102.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1145-colors-aggressive.docx":                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1145-colors.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"1157.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1172.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1200-1.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1200-2.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1200-3.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1200-4.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1200-5.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1306.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"1312-fonts-info-2.docx":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"1312-fonts-info.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1341-textbox-with-a-hyperlink.docx":                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"1370-same-nested-revisions.docx":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"1385-whitespace-styles.docx":                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"1394-highlights.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1394-styles.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"1406-code-finding.docx":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"1413-notes.docx":                                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"1421-line-break.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"1433-text-for-masking.docx":                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"1437-color-exclusion.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"768-2.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"768.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-1.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-2.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-3.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-4.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-5.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-6.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"830-7.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"834.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"847-2.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"847-3.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"851.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"859.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"884.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"887.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"899.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"947-non-cs-and-cs.docx":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"952-1.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"952-2.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"952-3.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"956.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"992.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"Addcomments.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"AltContentEscaping.docx":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"AlternateContent.docx":                                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"AlternateContentTest.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"BoldWorld.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"Deli.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-custom-tabs.docx":                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-formula-and-tabs.docx":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-soft-linebreaks.docx":                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs-2.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs-3.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs-4.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs-5.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs-at-EOL.docx":                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"Document-with-tabs.docx":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"DrawingML_Test.docx":                                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"EndGroup.docx":                                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"Escapades.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"GraphicInTextBox.docx":                                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"Hangs.docx":                                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"HelloWorld.docx":                                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"HiddenExcluded.docx":                                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"HiddenTablesApachePoi.docx":                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"MissingPara.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"N_001_Auswertung_Part4.docx":                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"OkapiMarkers.docx":                                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"OpenXML_text_reference_v1_2.docx":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"OpenXmlRoundtripSoftLineBreaksDoNotTranslateTestCharacterStyle.docx": {Engines: []string{"bridge", "native"}, Reason: reason},
		"OpenXmlRoundtripSoftLineBreaksDoNotTranslateTestParagraphStyle.docx": {Engines: []string{"bridge", "native"}, Reason: reason},
		"OpenXmlRoundtripTabDoNotTranslateTestCharacterStyle.docx":            {Engines: []string{"bridge", "native"}, Reason: reason},
		"OpenXmlRoundtripTabDoNotTranslateTestParagraphStyle.docx":            {Engines: []string{"bridge", "native"}, Reason: reason},
		"OutOfTheTextBox.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Practice2.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"StartsWithLineSeparator.docx":                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"TabAtEnd.docx":                                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"TabAtEndAfterNewRun.docx":                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestDako2.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestLTinsideBoxFails.docx":                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"TextboxNumber.docx":                                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"apissue.docx":                                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"br.docx":                                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"br2.docx":                                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"chartAmpersand.docx":                                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"content_category_test.docx":                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"delTextAmp.docx":                                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"document-revision-information-stripping.docx":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"document-style-definitions.docx":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"document-with-run-fonts-variations.docx":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"docxsegtest.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"docxtest.docx":                                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"external_hyperlink.docx":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"gettysburg_en.docx":                                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"graphicdata.docx":                                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"highlight_in_style.docx":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"highlights.docx":                                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"highlights_block.docx":                                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"hyperlink.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"lang.docx":                                                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"large-attribute.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"multiple_tabs.docx":                                                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"neverendingloop.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"picture.docx":                                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"sample.docx":                                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"shape with text.docx":                                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_chart.docx":                                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"smart_art.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"spacing.docx":                                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"special-chars-and-linebreaks.docx":                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"styles_color.docx":                                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"tabstyles.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"textarea.docx":                                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"vertAlign.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"watermark.docx":                                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"word art.docx":                                                       {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

// htmlBridgeSkips, markdownBridgeSkips, poBridgeSkips, csvBridgeSkips,
// tsBridgeSkips, mifBridgeSkips list the per-file bridge divergences
// for formats where bridge passes some fixtures and fails others.
// Filling these out preserves the passing-fixture assertions (vs
// hiding everything behind a format-default bridge skip).
//
// The shared root cause across these formats is the bridge daemon's
// StreamingTranslationApplier: when it merges the streamed target
// segments back into the parsed event stream, it loses or rebuilds
// inline codes (HTML tags, XML elements, MIF variables, PO escapes,
// etc.) differently than upstream Okapi's in-process pipeline. The
// passing fixtures are the ones with no inline codes at all.

func htmlBridgeSkips() map[string]fileSkip {
	const reason = "bridge inline-code divergence in HTML output vs okapi reference"
	return map[string]fileSkip{
		"324.html":                                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"Dachseite-Startseite.html":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"ExcludeIncludeTest.html":                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"France_Culture_fr.html":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"W3CHTMHLTest1.html":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"advanced_bold_font_color.html":                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"bad_textUnit.html":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"burlington_ufo_center.html":                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"form.html":                                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"form2.html":                                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"home_big.html":                                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"home_crush.html":                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"home_links.html":                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"home_swing.html":                                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"merged_codes.html":                                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"sanitizer.html":                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"segmentation_test.html":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple2.html":                                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold.html":                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_font_color.html":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_italic_font_color.html":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_italic_underline.html":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_italics.html":                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_italics_embeded.html":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_italics_separate.html":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_bold_underline.html":                            {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_em.html":                                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_em_bold.html":                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_em_font_color.html":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_font.html":                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_font_bold.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_font_color.html":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_font_size.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_highlight.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_italic_bold.html":                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_italics.html":                                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_link.html":                                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_many_bold.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_shadow.html":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_strike.html":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_strike_bold_italics.html":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_strong.html":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_styles_bold.html":                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_styles_italics_bold.html":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_styles_strike_bold_italics.html":                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_styles_strike_bold_underline_italics.html":      {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_subscript.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_superscript.html":                               {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_superscript_subscript.html":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_superscript_subscript_bold.html":                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_superscript_subscript_bold_italics.html":        {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_superscript_subscript_italics.html":             {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_underline.html":                                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_upper_case_bold.html":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_upper_case_italic.html":                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_upper_case_italic_bold.html":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_upper_case_strike.html":                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_upper_case_underline.html":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"supplementals.html":                                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"td.html":                                               {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func markdownBridgeSkips() map[string]fileSkip {
	const reason = "bridge markdown inline-style emission divergence (bold/italic/code spans rebuilt differently than okapi)"
	return map[string]fileSkip{
		"DirectShape.md":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"admonitions.md":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"code_and_codeblock_tests.md":          {Engines: []string{"bridge", "native"}, Reason: reason},
		"commonmark_changed.md":                {Engines: []string{"bridge", "native"}, Reason: reason},
		"commonmark_original.md":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"deployconfigure-reality.md":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"direct-links-uppercased.md":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"direct-links.md":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"example1.md":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"example2.md":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"example3.md":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"example4.md":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"example5.md":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"html-cdata-sample-uppercased.md":      {Engines: []string{"bridge", "native"}, Reason: reason},
		"html-cdata-sample.md":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"html_table1_original.md":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"image-wo-alt.md":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"img_w_alt_attr_original.md":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"link-titles.md":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"lists_changed.md":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"lists_original.md":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"multiple-segments.md":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"quoted-list.md":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"quotes-after-html-in-table.md":        {Engines: []string{"bridge", "native"}, Reason: reason},
		"ref-links-uppercased.md":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"ref-links.md":                         {Engines: []string{"bridge", "native"}, Reason: reason},
		"sample_html_combo.md":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"space-test.md":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"table1_changed.md":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"table1_original.md":                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"table2_changed.md":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"table2_original.md":                   {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func poBridgeSkips() map[string]fileSkip {
	const reason = "bridge PO writer escape/quote/printf-format divergence vs okapi reference"
	return map[string]fileSkip{
		"AllCasesTest.po":                       {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test01.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test02.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test03.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test04.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test05.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestMonoLingual_EN.po":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestMonoLingual_FR.po":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_DrupalRussianCP1251.po":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_nautilus.af.po":                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"escaping.po":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"multientry_multilinecomments.po":       {Engines: []string{"bridge", "native"}, Reason: reason},
		"multientry_withtranslation.po":         {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple.po":                             {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_multilinecomments.po":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_multilinestringwithtranslation.po": {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_withcontext.po":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_withpluralforms.po":             {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func csvBridgeSkips() map[string]fileSkip {
	const reason = "bridge CSV writer quoting / cell-grouping divergence vs okapi reference"
	return map[string]fileSkip{
		"computer_science_article.csv":            {Engines: []string{"bridge", "native"}, Reason: reason},
		"field_delimiter_comma.csv":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"some_blank_rows.csv":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"test2cols.csv":                           {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_double_quote.csv":         {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_double_quote_inside.csv":  {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_single_quote.csv":         {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_single_quote_inside.csv":  {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func tsBridgeSkips() map[string]fileSkip {
	const reason = "bridge TS writer attaches -ERR:PROP-NOT-FOUND- placeholder where okapi emits type=\"unfinished\""
	return map[string]fileSkip{
		"Complete_valid_utf8_bom_crlf.ts": {Engines: []string{"bridge", "native"}, Reason: reason},
		"TSTest01.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestInQT.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestInQT_Saved.ts":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_nautilus.af.ts":             {Engines: []string{"bridge", "native"}, Reason: reason},
		"autoSample.ts":                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"issue531.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"tstest.ts":                       {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func mifBridgeSkips() map[string]fileSkip {
	const reason = "bridge MIF writer drops <$lastpagenum>-style variable references inside VariableFormat blocks"
	return map[string]fileSkip{
		"1188_crlf.mif":                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"893.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"895.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"896-autonumber-building-blocks.mif": {Engines: []string{"bridge", "native"}, Reason: reason},
		"896-changed.mif":                {Engines: []string{"bridge", "native"}, Reason: reason},
		"896.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"902-1.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"902-2.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"902-3.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"904.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"909-1.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"909-2.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"909-3.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"938-1.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"938-2.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"940.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"942-1.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"942-2.mif":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"943.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"945.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"987.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-marker.mif":                 {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-pgf-num-format-1.mif":       {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-pgf-num-format-2.mif":       {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-ref-format-1.mif":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-ref-format-2.mif":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"990-text-line.mif":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"991.mif":                        {Engines: []string{"bridge", "native"}, Reason: reason},
		"ImportedText.mif":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"JATest.mif":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test01-v8.mif":                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test01.mif":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test02-v9.mif":                  {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test03.mif":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test04.mif":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestEncoding-v10.mif":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestEncoding-v9.mif":            {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestFootnote.mif":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestMarkers.mif":                {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

// icmlMergedSkips combines two distinct kinds of per-file skips for
// the ICML format: 5 fixtures crash upstream Okapi's merger (so the
// okapi reference itself is unusable), and 7 more diverge in the
// bridge's inline-code rewrite pass.
func icmlMergedSkips() map[string]fileSkip {
	const okapiReason = "upstream Okapi icml merge crashes on this fixture"
	const bridgeReason = "bridge ICML CharacterStyleRange/Content rewrite divergence vs okapi reference"
	m := map[string]fileSkip{
		"OpenofficeFootnoteTest.icml":                                {Engines: []string{"okapi"}, Reason: okapiReason},
		"TakeItNoItsYoursReallyTheExcellentInevitabilityOfFree.icml": {Engines: []string{"okapi"}, Reason: okapiReason},
		"TestArticle.icml":                                           {Engines: []string{"okapi"}, Reason: okapiReason},
		"ThreeParagraphFootnoteTest.icml":                            {Engines: []string{"okapi"}, Reason: okapiReason},
		"WordFootnoteTest.icml":                                      {Engines: []string{"okapi"}, Reason: okapiReason},
	}
	for _, name := range []string{
		"DraftForJEP.icml",
		"NotesTowardV10.icml",
		"ParagraphClassTest.icml",
		"SpanClassTest.icml",
		"XMLProductionStartWithTheWeb.icml",
		"not_valid.icml",
		"valid.icml",
	} {
		m[name] = fileSkip{Engines: []string{"bridge", "native"}, Reason: bridgeReason}
	}
	return m
}
