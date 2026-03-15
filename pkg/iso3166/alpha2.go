package iso3166

import "strings"

// ValidAlpha2 returns true if the given string is a valid ISO 3166-1 alpha-2 country code.
// The input is normalized to uppercase before lookup; exactly two characters are required.
// This package contains no business logic, only the set of assigned codes.
func ValidAlpha2(cc string) bool {
	if len(cc) != 2 {
		return false
	}
	_, ok := alpha2Set[strings.ToUpper(cc)]
	return ok
}

// alpha2Set contains all ISO 3166-1 alpha-2 assigned country codes (uppercase).
var alpha2Set = map[string]struct{}{
	"AD": {}, "AE": {}, "AF": {}, "AG": {}, "AI": {}, "AL": {}, "AM": {}, "AO": {}, "AQ": {}, "AR": {},
	"AS": {}, "AT": {}, "AU": {}, "AW": {}, "AX": {}, "AZ": {}, "BA": {}, "BB": {}, "BD": {}, "BE": {},
	"BF": {}, "BG": {}, "BH": {}, "BI": {}, "BJ": {}, "BL": {}, "BM": {}, "BN": {}, "BO": {}, "BQ": {},
	"BR": {}, "BS": {}, "BT": {}, "BV": {}, "BW": {}, "BY": {}, "BZ": {}, "CA": {}, "CC": {}, "CD": {},
	"CF": {}, "CG": {}, "CH": {}, "CI": {}, "CK": {}, "CL": {}, "CM": {}, "CN": {}, "CO": {}, "CR": {},
	"CU": {}, "CV": {}, "CW": {}, "CX": {}, "CY": {}, "CZ": {}, "DE": {}, "DJ": {}, "DK": {}, "DM": {},
	"DO": {}, "DZ": {}, "EC": {}, "EE": {}, "EG": {}, "EH": {}, "ER": {}, "ES": {}, "ET": {}, "FI": {},
	"FJ": {}, "FK": {}, "FM": {}, "FO": {}, "FR": {}, "GA": {}, "GB": {}, "GD": {}, "GE": {}, "GF": {},
	"GG": {}, "GH": {}, "GI": {}, "GL": {}, "GM": {}, "GN": {}, "GP": {}, "GQ": {}, "GR": {}, "GS": {},
	"GT": {}, "GU": {}, "GW": {}, "GY": {}, "HK": {}, "HM": {}, "HN": {}, "HR": {}, "HT": {}, "HU": {},
	"ID": {}, "IE": {}, "IL": {}, "IM": {}, "IN": {}, "IO": {}, "IQ": {}, "IR": {}, "IS": {}, "IT": {},
	"JE": {}, "JM": {}, "JO": {}, "JP": {}, "KE": {}, "KG": {}, "KH": {}, "KI": {}, "KM": {}, "KN": {},
	"KP": {}, "KR": {}, "KW": {}, "KY": {}, "KZ": {}, "LA": {}, "LB": {}, "LC": {}, "LI": {}, "LK": {},
	"LR": {}, "LS": {}, "LT": {}, "LU": {}, "LV": {}, "LY": {}, "MA": {}, "MC": {}, "MD": {}, "ME": {},
	// ZZ and QQ are not assigned in ISO 3166-1 alpha-2; omitted so ValidAlpha2 returns false.
	"MF": {}, "MG": {}, "MH": {}, "MK": {}, "ML": {}, "MM": {}, "MN": {}, "MO": {}, "MP": {}, "MQ": {},
	"MR": {}, "MS": {}, "MT": {}, "MU": {}, "MV": {}, "MW": {}, "MX": {}, "MY": {}, "MZ": {}, "NA": {},
	"NC": {}, "NE": {}, "NF": {}, "NG": {}, "NI": {}, "NL": {}, "NO": {}, "NP": {}, "NR": {}, "NU": {},
	"NZ": {}, "OM": {}, "PA": {}, "PE": {}, "PF": {}, "PG": {}, "PH": {}, "PK": {}, "PL": {}, "PM": {},
	"PN": {}, "PR": {}, "PS": {}, "PT": {}, "PW": {}, "PY": {}, "QA": {}, "RE": {}, "RO": {}, "RS": {},
	"RU": {}, "RW": {}, "SA": {}, "SB": {}, "SC": {}, "SD": {}, "SE": {}, "SG": {}, "SH": {}, "SI": {},
	"SJ": {}, "SK": {}, "SL": {}, "SM": {}, "SN": {}, "SO": {}, "SR": {}, "SS": {}, "ST": {}, "SV": {},
	"SX": {}, "SY": {}, "SZ": {}, "TC": {}, "TD": {}, "TF": {}, "TG": {}, "TH": {}, "TJ": {}, "TK": {},
	"TL": {}, "TM": {}, "TN": {}, "TO": {}, "TR": {}, "TT": {}, "TV": {}, "TW": {}, "TZ": {}, "UA": {},
	"UG": {}, "UM": {}, "US": {}, "UY": {}, "UZ": {}, "VA": {}, "VC": {}, "VE": {}, "VG": {}, "VI": {},
	"VN": {}, "VU": {}, "WF": {}, "WS": {}, "YE": {}, "YT": {}, "ZA": {}, "ZM": {}, "ZW": {},
}
