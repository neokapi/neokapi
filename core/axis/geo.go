package axis

import "strings"

// World is the geo value that contains every country and region.
const World = "world"

// Place is one country or territory of ISO 3166-1, as a geo axis holds it.
type Place struct {
	// Code is the ISO 3166-1 alpha-2 code in lower case: "no", "fr".
	Code string
	// Name is the English short name.
	Name string
	// Lat and Lon are an approximate centroid, in degrees, for drawing the
	// place on a map. They are not boundaries.
	Lat, Lon float64
	// Region is the UN M49 region the place sits in, as a slug: the
	// intermediate region where M49 names one ("caribbean"), else the
	// subregion ("northern-europe"). Antarctica sits directly in World.
	Region string
}

// Region is one UN M49 region, subregion or intermediate region.
type Region struct {
	// Slug is the region's name in lower case with hyphens:
	// "latin-america-and-the-caribbean".
	Slug string
	// Name is the English name.
	Name string
	// Code is the three-digit M49 code, which a locale may carry as its region
	// subtag (es-419).
	Code string
	// Parent is the region containing this one, World for a continent.
	Parent string
}

// regions are the UN M49 regions, subregions and intermediate regions, in the
// order M49 lists them.
var regions = []Region{
	{"africa", "Africa", "002", World},
	{"northern-africa", "Northern Africa", "015", "africa"},
	{"sub-saharan-africa", "Sub-Saharan Africa", "202", "africa"},
	{"eastern-africa", "Eastern Africa", "014", "sub-saharan-africa"},
	{"middle-africa", "Middle Africa", "017", "sub-saharan-africa"},
	{"southern-africa", "Southern Africa", "018", "sub-saharan-africa"},
	{"western-africa", "Western Africa", "011", "sub-saharan-africa"},
	{"americas", "Americas", "019", World},
	{"latin-america-and-the-caribbean", "Latin America and the Caribbean", "419", "americas"},
	{"caribbean", "Caribbean", "029", "latin-america-and-the-caribbean"},
	{"central-america", "Central America", "013", "latin-america-and-the-caribbean"},
	{"south-america", "South America", "005", "latin-america-and-the-caribbean"},
	{"northern-america", "Northern America", "021", "americas"},
	{"asia", "Asia", "142", World},
	{"central-asia", "Central Asia", "143", "asia"},
	{"eastern-asia", "Eastern Asia", "030", "asia"},
	{"south-eastern-asia", "South-eastern Asia", "035", "asia"},
	{"southern-asia", "Southern Asia", "034", "asia"},
	{"western-asia", "Western Asia", "145", "asia"},
	{"europe", "Europe", "150", World},
	{"eastern-europe", "Eastern Europe", "151", "europe"},
	{"northern-europe", "Northern Europe", "154", "europe"},
	{"southern-europe", "Southern Europe", "039", "europe"},
	{"western-europe", "Western Europe", "155", "europe"},
	{"oceania", "Oceania", "009", World},
	{"australia-and-new-zealand", "Australia and New Zealand", "053", "oceania"},
	{"melanesia", "Melanesia", "054", "oceania"},
	{"micronesia", "Micronesia", "057", "oceania"},
	{"polynesia", "Polynesia", "061", "oceania"},
}

// places are the countries and territories of ISO 3166-1, with an
// approximate centroid and the M49 region each sits in.
var places = []Place{
	{"ad", "Andorra", 42.55, 1.60, "southern-europe"},
	{"ae", "United Arab Emirates", 23.42, 53.85, "western-asia"},
	{"af", "Afghanistan", 33.94, 67.71, "southern-asia"},
	{"ag", "Antigua and Barbuda", 17.06, -61.80, "caribbean"},
	{"ai", "Anguilla", 18.22, -63.07, "caribbean"},
	{"al", "Albania", 41.15, 20.17, "southern-europe"},
	{"am", "Armenia", 40.07, 45.04, "western-asia"},
	{"ao", "Angola", -11.20, 17.87, "middle-africa"},
	{"aq", "Antarctica", -75.25, -0.07, World},
	{"ar", "Argentina", -38.42, -63.62, "south-america"},
	{"as", "American Samoa", -14.27, -170.13, "polynesia"},
	{"at", "Austria", 47.52, 14.55, "western-europe"},
	{"au", "Australia", -25.27, 133.78, "australia-and-new-zealand"},
	{"aw", "Aruba", 12.52, -69.97, "caribbean"},
	{"ax", "Åland Islands", 60.18, 19.92, "northern-europe"},
	{"az", "Azerbaijan", 40.14, 47.58, "western-asia"},
	{"ba", "Bosnia and Herzegovina", 43.92, 17.68, "southern-europe"},
	{"bb", "Barbados", 13.19, -59.54, "caribbean"},
	{"bd", "Bangladesh", 23.68, 90.36, "southern-asia"},
	{"be", "Belgium", 50.50, 4.47, "western-europe"},
	{"bf", "Burkina Faso", 12.24, -1.56, "western-africa"},
	{"bg", "Bulgaria", 42.73, 25.49, "eastern-europe"},
	{"bh", "Bahrain", 25.93, 50.64, "western-asia"},
	{"bi", "Burundi", -3.37, 29.92, "eastern-africa"},
	{"bj", "Benin", 9.31, 2.32, "western-africa"},
	{"bl", "Saint Barthélemy", 17.90, -62.83, "caribbean"},
	{"bm", "Bermuda", 32.32, -64.76, "northern-america"},
	{"bn", "Brunei Darussalam", 4.54, 114.73, "south-eastern-asia"},
	{"bo", "Bolivia", -16.29, -63.59, "south-america"},
	{"bq", "Bonaire, Sint Eustatius and Saba", 12.18, -68.26, "caribbean"},
	{"br", "Brazil", -14.24, -51.93, "south-america"},
	{"bs", "Bahamas", 25.03, -77.40, "caribbean"},
	{"bt", "Bhutan", 27.51, 90.43, "southern-asia"},
	{"bv", "Bouvet Island", -54.42, 3.41, "south-america"},
	{"bw", "Botswana", -22.33, 24.68, "southern-africa"},
	{"by", "Belarus", 53.71, 27.95, "eastern-europe"},
	{"bz", "Belize", 17.19, -88.50, "central-america"},
	{"ca", "Canada", 56.13, -106.35, "northern-america"},
	{"cc", "Cocos (Keeling) Islands", -12.16, 96.87, "australia-and-new-zealand"},
	{"cd", "Democratic Republic of the Congo", -4.04, 21.76, "middle-africa"},
	{"cf", "Central African Republic", 6.61, 20.94, "middle-africa"},
	{"cg", "Congo", -0.23, 15.83, "middle-africa"},
	{"ch", "Switzerland", 46.82, 8.23, "western-europe"},
	{"ci", "Côte d'Ivoire", 7.54, -5.55, "western-africa"},
	{"ck", "Cook Islands", -21.24, -159.78, "polynesia"},
	{"cl", "Chile", -35.68, -71.54, "south-america"},
	{"cm", "Cameroon", 7.37, 12.35, "middle-africa"},
	{"cn", "China", 35.86, 104.20, "eastern-asia"},
	{"co", "Colombia", 4.57, -74.30, "south-america"},
	{"cr", "Costa Rica", 9.75, -83.75, "central-america"},
	{"cu", "Cuba", 21.52, -77.78, "caribbean"},
	{"cv", "Cabo Verde", 16.00, -24.01, "western-africa"},
	{"cw", "Curaçao", 12.17, -68.99, "caribbean"},
	{"cx", "Christmas Island", -10.45, 105.69, "australia-and-new-zealand"},
	{"cy", "Cyprus", 35.13, 33.43, "western-asia"},
	{"cz", "Czechia", 49.82, 15.47, "eastern-europe"},
	{"de", "Germany", 51.17, 10.45, "western-europe"},
	{"dj", "Djibouti", 11.83, 42.59, "eastern-africa"},
	{"dk", "Denmark", 56.26, 9.50, "northern-europe"},
	{"dm", "Dominica", 15.41, -61.37, "caribbean"},
	{"do", "Dominican Republic", 18.74, -70.16, "caribbean"},
	{"dz", "Algeria", 28.03, 1.66, "northern-africa"},
	{"ec", "Ecuador", -1.83, -78.18, "south-america"},
	{"ee", "Estonia", 58.60, 25.01, "northern-europe"},
	{"eg", "Egypt", 26.82, 30.80, "northern-africa"},
	{"eh", "Western Sahara", 24.22, -12.89, "northern-africa"},
	{"er", "Eritrea", 15.18, 39.78, "eastern-africa"},
	{"es", "Spain", 40.46, -3.75, "southern-europe"},
	{"et", "Ethiopia", 9.15, 40.49, "eastern-africa"},
	{"fi", "Finland", 61.92, 25.75, "northern-europe"},
	{"fj", "Fiji", -16.58, 179.41, "melanesia"},
	{"fk", "Falkland Islands (Malvinas)", -51.80, -59.52, "south-america"},
	{"fm", "Micronesia (Federated States of)", 7.43, 150.55, "micronesia"},
	{"fo", "Faroe Islands", 61.89, -6.91, "northern-europe"},
	{"fr", "France", 46.23, 2.21, "western-europe"},
	{"ga", "Gabon", -0.80, 11.61, "middle-africa"},
	{"gb", "United Kingdom", 55.38, -3.44, "northern-europe"},
	{"gd", "Grenada", 12.26, -61.60, "caribbean"},
	{"ge", "Georgia", 42.32, 43.36, "western-asia"},
	{"gf", "French Guiana", 3.93, -53.13, "south-america"},
	{"gg", "Guernsey", 49.47, -2.59, "northern-europe"},
	{"gh", "Ghana", 7.95, -1.02, "western-africa"},
	{"gi", "Gibraltar", 36.14, -5.35, "southern-europe"},
	{"gl", "Greenland", 71.71, -42.60, "northern-america"},
	{"gm", "Gambia", 13.44, -15.31, "western-africa"},
	{"gn", "Guinea", 9.95, -9.70, "western-africa"},
	{"gp", "Guadeloupe", 16.25, -61.58, "caribbean"},
	{"gq", "Equatorial Guinea", 1.65, 10.27, "middle-africa"},
	{"gr", "Greece", 39.07, 21.82, "southern-europe"},
	{"gs", "South Georgia and the South Sandwich Islands", -54.43, -36.59, "south-america"},
	{"gt", "Guatemala", 15.78, -90.23, "central-america"},
	{"gu", "Guam", 13.44, 144.79, "micronesia"},
	{"gw", "Guinea-Bissau", 11.80, -15.18, "western-africa"},
	{"gy", "Guyana", 4.86, -58.93, "south-america"},
	{"hk", "Hong Kong", 22.40, 114.11, "eastern-asia"},
	{"hm", "Heard Island and McDonald Islands", -53.08, 73.50, "australia-and-new-zealand"},
	{"hn", "Honduras", 15.20, -86.24, "central-america"},
	{"hr", "Croatia", 45.10, 15.20, "southern-europe"},
	{"ht", "Haiti", 18.97, -72.29, "caribbean"},
	{"hu", "Hungary", 47.16, 19.50, "eastern-europe"},
	{"id", "Indonesia", -0.79, 113.92, "south-eastern-asia"},
	{"ie", "Ireland", 53.41, -8.24, "northern-europe"},
	{"il", "Israel", 31.05, 34.85, "western-asia"},
	{"im", "Isle of Man", 54.24, -4.55, "northern-europe"},
	{"in", "India", 20.59, 78.96, "southern-asia"},
	{"io", "British Indian Ocean Territory", -6.34, 71.88, "eastern-africa"},
	{"iq", "Iraq", 33.22, 43.68, "western-asia"},
	{"ir", "Iran", 32.43, 53.69, "southern-asia"},
	{"is", "Iceland", 64.96, -19.02, "northern-europe"},
	{"it", "Italy", 41.87, 12.57, "southern-europe"},
	{"je", "Jersey", 49.21, -2.13, "northern-europe"},
	{"jm", "Jamaica", 18.11, -77.30, "caribbean"},
	{"jo", "Jordan", 30.59, 36.24, "western-asia"},
	{"jp", "Japan", 36.20, 138.25, "eastern-asia"},
	{"ke", "Kenya", -0.02, 37.91, "eastern-africa"},
	{"kg", "Kyrgyzstan", 41.20, 74.77, "central-asia"},
	{"kh", "Cambodia", 12.57, 104.99, "south-eastern-asia"},
	{"ki", "Kiribati", 1.87, -157.36, "micronesia"},
	{"km", "Comoros", -11.88, 43.87, "eastern-africa"},
	{"kn", "Saint Kitts and Nevis", 17.36, -62.78, "caribbean"},
	{"kp", "North Korea", 40.34, 127.51, "eastern-asia"},
	{"kr", "South Korea", 35.91, 127.77, "eastern-asia"},
	{"kw", "Kuwait", 29.31, 47.48, "western-asia"},
	{"ky", "Cayman Islands", 19.51, -80.57, "caribbean"},
	{"kz", "Kazakhstan", 48.02, 66.92, "central-asia"},
	{"la", "Lao People's Democratic Republic", 19.86, 102.50, "south-eastern-asia"},
	{"lb", "Lebanon", 33.85, 35.86, "western-asia"},
	{"lc", "Saint Lucia", 13.91, -60.98, "caribbean"},
	{"li", "Liechtenstein", 47.17, 9.56, "western-europe"},
	{"lk", "Sri Lanka", 7.87, 80.77, "southern-asia"},
	{"lr", "Liberia", 6.43, -9.43, "western-africa"},
	{"ls", "Lesotho", -29.61, 28.23, "southern-africa"},
	{"lt", "Lithuania", 55.17, 23.88, "northern-europe"},
	{"lu", "Luxembourg", 49.82, 6.13, "western-europe"},
	{"lv", "Latvia", 56.88, 24.60, "northern-europe"},
	{"ly", "Libya", 26.34, 17.23, "northern-africa"},
	{"ma", "Morocco", 31.79, -7.09, "northern-africa"},
	{"mc", "Monaco", 43.75, 7.41, "western-europe"},
	{"md", "Moldova", 47.41, 28.37, "eastern-europe"},
	{"me", "Montenegro", 42.71, 19.37, "southern-europe"},
	{"mf", "Saint Martin (French part)", 18.08, -63.05, "caribbean"},
	{"mg", "Madagascar", -18.77, 46.87, "eastern-africa"},
	{"mh", "Marshall Islands", 7.13, 171.18, "micronesia"},
	{"mk", "North Macedonia", 41.61, 21.75, "southern-europe"},
	{"ml", "Mali", 17.57, -4.00, "western-africa"},
	{"mm", "Myanmar", 21.91, 95.96, "south-eastern-asia"},
	{"mn", "Mongolia", 46.86, 103.85, "eastern-asia"},
	{"mo", "Macao", 22.20, 113.54, "eastern-asia"},
	{"mp", "Northern Mariana Islands", 17.33, 145.38, "micronesia"},
	{"mq", "Martinique", 14.64, -61.02, "caribbean"},
	{"mr", "Mauritania", 21.01, -10.94, "western-africa"},
	{"ms", "Montserrat", 16.74, -62.19, "caribbean"},
	{"mt", "Malta", 35.94, 14.38, "southern-europe"},
	{"mu", "Mauritius", -20.35, 57.55, "eastern-africa"},
	{"mv", "Maldives", 3.20, 73.22, "southern-asia"},
	{"mw", "Malawi", -13.25, 34.30, "eastern-africa"},
	{"mx", "Mexico", 23.63, -102.55, "central-america"},
	{"my", "Malaysia", 4.21, 101.98, "south-eastern-asia"},
	{"mz", "Mozambique", -18.67, 35.53, "eastern-africa"},
	{"na", "Namibia", -22.96, 18.49, "southern-africa"},
	{"nc", "New Caledonia", -20.90, 165.62, "melanesia"},
	{"ne", "Niger", 17.61, 8.08, "western-africa"},
	{"nf", "Norfolk Island", -29.04, 167.95, "australia-and-new-zealand"},
	{"ng", "Nigeria", 9.08, 8.68, "western-africa"},
	{"ni", "Nicaragua", 12.87, -85.21, "central-america"},
	{"nl", "Netherlands", 52.13, 5.29, "western-europe"},
	{"no", "Norway", 60.47, 8.47, "northern-europe"},
	{"np", "Nepal", 28.39, 84.12, "southern-asia"},
	{"nr", "Nauru", -0.52, 166.93, "micronesia"},
	{"nu", "Niue", -19.05, -169.87, "polynesia"},
	{"nz", "New Zealand", -40.90, 174.89, "australia-and-new-zealand"},
	{"om", "Oman", 21.51, 55.92, "western-asia"},
	{"pa", "Panama", 8.54, -80.78, "central-america"},
	{"pe", "Peru", -9.19, -75.02, "south-america"},
	{"pf", "French Polynesia", -17.68, -149.41, "polynesia"},
	{"pg", "Papua New Guinea", -6.31, 143.96, "melanesia"},
	{"ph", "Philippines", 12.88, 121.77, "south-eastern-asia"},
	{"pk", "Pakistan", 30.38, 69.35, "southern-asia"},
	{"pl", "Poland", 51.92, 19.15, "eastern-europe"},
	{"pm", "Saint Pierre and Miquelon", 46.94, -56.27, "northern-america"},
	{"pn", "Pitcairn", -24.70, -127.44, "polynesia"},
	{"pr", "Puerto Rico", 18.22, -66.59, "caribbean"},
	{"ps", "Palestine, State of", 31.95, 35.23, "western-asia"},
	{"pt", "Portugal", 39.40, -8.22, "southern-europe"},
	{"pw", "Palau", 7.51, 134.58, "micronesia"},
	{"py", "Paraguay", -23.44, -58.44, "south-america"},
	{"qa", "Qatar", 25.35, 51.18, "western-asia"},
	{"re", "Réunion", -21.12, 55.54, "eastern-africa"},
	{"ro", "Romania", 45.94, 24.97, "eastern-europe"},
	{"rs", "Serbia", 44.02, 21.01, "southern-europe"},
	{"ru", "Russian Federation", 61.52, 105.32, "eastern-europe"},
	{"rw", "Rwanda", -1.94, 29.87, "eastern-africa"},
	{"sa", "Saudi Arabia", 23.89, 45.08, "western-asia"},
	{"sb", "Solomon Islands", -9.65, 160.16, "melanesia"},
	{"sc", "Seychelles", -4.68, 55.49, "eastern-africa"},
	{"sd", "Sudan", 12.86, 30.22, "northern-africa"},
	{"se", "Sweden", 60.13, 18.64, "northern-europe"},
	{"sg", "Singapore", 1.35, 103.82, "south-eastern-asia"},
	{"sh", "Saint Helena, Ascension and Tristan da Cunha", -24.14, -10.03, "western-africa"},
	{"si", "Slovenia", 46.15, 15.00, "southern-europe"},
	{"sj", "Svalbard and Jan Mayen", 77.55, 23.67, "northern-europe"},
	{"sk", "Slovakia", 48.67, 19.70, "eastern-europe"},
	{"sl", "Sierra Leone", 8.46, -11.78, "western-africa"},
	{"sm", "San Marino", 43.94, 12.46, "southern-europe"},
	{"sn", "Senegal", 14.50, -14.45, "western-africa"},
	{"so", "Somalia", 5.15, 46.20, "eastern-africa"},
	{"sr", "Suriname", 3.92, -56.03, "south-america"},
	{"ss", "South Sudan", 6.88, 31.31, "eastern-africa"},
	{"st", "Sao Tome and Principe", 0.19, 6.61, "middle-africa"},
	{"sv", "El Salvador", 13.79, -88.90, "central-america"},
	{"sx", "Sint Maarten (Dutch part)", 18.04, -63.07, "caribbean"},
	{"sy", "Syrian Arab Republic", 34.80, 39.00, "western-asia"},
	{"sz", "Eswatini", -26.52, 31.47, "southern-africa"},
	{"tc", "Turks and Caicos Islands", 21.69, -71.80, "caribbean"},
	{"td", "Chad", 15.45, 18.73, "middle-africa"},
	{"tf", "French Southern Territories", -49.28, 69.35, "eastern-africa"},
	{"tg", "Togo", 8.62, 0.82, "western-africa"},
	{"th", "Thailand", 15.87, 100.99, "south-eastern-asia"},
	{"tj", "Tajikistan", 38.86, 71.28, "central-asia"},
	{"tk", "Tokelau", -8.97, -171.86, "polynesia"},
	{"tl", "Timor-Leste", -8.87, 125.73, "south-eastern-asia"},
	{"tm", "Turkmenistan", 38.97, 59.56, "central-asia"},
	{"tn", "Tunisia", 33.89, 9.54, "northern-africa"},
	{"to", "Tonga", -21.18, -175.20, "polynesia"},
	{"tr", "Türkiye", 38.96, 35.24, "western-asia"},
	{"tt", "Trinidad and Tobago", 10.69, -61.22, "caribbean"},
	{"tv", "Tuvalu", -7.11, 177.65, "polynesia"},
	{"tw", "Taiwan", 23.70, 120.96, "eastern-asia"},
	{"tz", "Tanzania", -6.37, 34.89, "eastern-africa"},
	{"ua", "Ukraine", 48.38, 31.17, "eastern-europe"},
	{"ug", "Uganda", 1.37, 32.29, "eastern-africa"},
	{"um", "United States Minor Outlying Islands", 19.28, 166.65, "micronesia"},
	{"us", "United States", 37.09, -95.71, "northern-america"},
	{"uy", "Uruguay", -32.52, -55.77, "south-america"},
	{"uz", "Uzbekistan", 41.38, 64.59, "central-asia"},
	{"va", "Holy See", 41.90, 12.45, "southern-europe"},
	{"vc", "Saint Vincent and the Grenadines", 12.98, -61.29, "caribbean"},
	{"ve", "Venezuela", 6.42, -66.59, "south-america"},
	{"vg", "Virgin Islands (British)", 18.42, -64.64, "caribbean"},
	{"vi", "Virgin Islands (U.S.)", 18.34, -64.90, "caribbean"},
	{"vn", "Viet Nam", 14.06, 108.28, "south-eastern-asia"},
	{"vu", "Vanuatu", -15.38, 166.96, "melanesia"},
	{"wf", "Wallis and Futuna", -13.77, -177.16, "polynesia"},
	{"ws", "Samoa", -13.76, -172.10, "polynesia"},
	{"ye", "Yemen", 15.55, 48.52, "western-asia"},
	{"yt", "Mayotte", -12.83, 45.17, "eastern-africa"},
	{"za", "South Africa", -30.56, 22.94, "southern-africa"},
	{"zm", "Zambia", -13.13, 27.85, "eastern-africa"},
	{"zw", "Zimbabwe", -19.02, 29.15, "eastern-africa"},
}

var (
	placeIndex  = map[string]Place{}
	regionIndex = map[string]Region{}
	regionCodes = map[string]string{}
)

func init() {
	for _, p := range places {
		placeIndex[p.Code] = p
	}
	for _, r := range regions {
		regionIndex[r.Slug] = r
		regionCodes[r.Code] = r.Slug
	}
}

// Places lists every country and territory the geo table holds, in code order.
func Places() []Place { return append([]Place(nil), places...) }

// Regions lists every M49 region, subregion and intermediate region, in the
// order M49 lists them.
func Regions() []Region { return append([]Region(nil), regions...) }

// PlaceOf returns the country or territory with an ISO 3166-1 alpha-2 code,
// in either case.
func PlaceOf(code string) (Place, bool) {
	p, ok := placeIndex[strings.ToLower(code)]
	return p, ok
}

// RegionOf returns the M49 region with a slug.
func RegionOf(slug string) (Region, bool) {
	r, ok := regionIndex[slug]
	return r, ok
}

// GeoParent is the value directly containing a built-in geo value: a
// country's region, a region's parent, empty for World and for a value the
// table does not hold.
func GeoParent(value string) string {
	if p, ok := placeIndex[value]; ok {
		return p.Region
	}
	if r, ok := regionIndex[value]; ok {
		return r.Parent
	}
	return ""
}

// GeoLabel is the English name of a built-in geo value, empty for a value the
// table does not hold.
func GeoLabel(value string) string {
	if p, ok := placeIndex[value]; ok {
		return p.Name
	}
	if r, ok := regionIndex[value]; ok {
		return r.Name
	}
	if value == World {
		return "World"
	}
	return ""
}

// knownGeo reports whether value is a country code, a region slug or World.
func knownGeo(value string) bool {
	if value == World {
		return true
	}
	if _, ok := placeIndex[value]; ok {
		return true
	}
	_, ok := regionIndex[value]
	return ok
}

// RegionOfLocale is the geo value a locale's region subtag names: the
// country code in lower case for fr-CA, the M49 region for es-419. A locale
// with no region subtag, or one the table does not hold, has none.
func RegionOfLocale(locale string) string {
	parts := strings.FieldsFunc(locale, func(r rune) bool { return r == '-' || r == '_' })
	for i, part := range parts {
		if i == 0 {
			continue
		}
		switch {
		case len(part) == 2 && isAlpha(part):
			if p, ok := placeIndex[strings.ToLower(part)]; ok {
				return p.Code
			}
			return ""
		case len(part) == 3 && isDigits(part):
			return regionCodes[part]
		}
	}
	return ""
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
