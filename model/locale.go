package model

// LocaleID represents a BCP 47 language tag.
type LocaleID string

const (
	LocaleEnglish    LocaleID = "en"
	LocaleFrench     LocaleID = "fr"
	LocaleGerman     LocaleID = "de"
	LocaleJapanese   LocaleID = "ja"
	LocaleSpanish    LocaleID = "es"
	LocaleChinese    LocaleID = "zh"
	LocalePortuguese LocaleID = "pt"
	LocaleItalian    LocaleID = "it"
	LocaleKorean     LocaleID = "ko"
	LocaleRussian    LocaleID = "ru"
	LocaleArabic     LocaleID = "ar"
)

// String returns the string representation of the LocaleID.
func (l LocaleID) String() string {
	return string(l)
}

// IsEmpty returns true if the locale is not set.
func (l LocaleID) IsEmpty() bool {
	return l == ""
}
