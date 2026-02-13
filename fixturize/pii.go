package fixturize

import (
	"slices"
	"strings"
)

type PIIRule struct {
	Category string
	Names    []string
	Types    []string
	Mask     string // SQL template: {pk} = PK column(s), {pki} = integer PK expression
}

var (
	textTypes    = []string{"character varying", "varchar", "text", "char", "citext"}
	numericTypes = []string{"integer", "bigint", "smallint", "numeric", "double precision", "real"}
)

var piiRules = []PIIRule{
	// identity
	{Category: "Email", Names: []string{"email", "emailaddress"}, Types: textTypes, Mask: "'user_' || {pk} || '@test.com'"},
	{Category: "Phone", Names: []string{"phone", "mobile", "telephone", "cell"}, Types: textTypes, Mask: "'+1555' || LPAD(({pki} % 10000000)::text, 7, '0')"},
	{Category: "First Name", Names: []string{"firstname", "fname", "given", "forename", "givenname", "jmeno", "prenom", "vorname", "voornaam"}, Types: textTypes, Mask: "'First' || {pk}"},
	{Category: "Last Name", Names: []string{"lastname", "lname", "surname", "familyname", "vorname", "nachname", "prijmeni"}, Types: textTypes, Mask: "'Last' || {pk}"},
	{Category: "Full Name", Names: []string{"fullname", "displayname"}, Types: textTypes, Mask: "'User ' || {pk}"},
	{Category: "Username", Names: []string{"username", "loginname", "screenname"}, Types: textTypes, Mask: "'user' || {pk}"},
	{Category: "Password", Names: []string{"password", "passwd", "pwd", "passhash", "pwhash"}, Types: textTypes, Mask: "'$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWX'"},
	{Category: "Date of Birth", Names: []string{"dob", "birthdate", "dateofbirth", "birthday", "geburtsdatum", "date_naissance", "datum_narozeni"}, Types: []string{"date", "timestamp"}, Mask: "'1990-01-01'::date + ({pki} % 10000)"},

	// Location
	{Category: "Address", Names: []string{"address", "street", "addr", "address1", "address2", "adresse", "strasse", "adresa", "ulice"}, Types: textTypes, Mask: "{pk} || ' Test Street'"},
	{Category: "City", Names: []string{"city", "town", "municipality", "stadt", "ville", "mesto"}, Types: textTypes, Mask: "'TestCity'"},
	{Category: "State", Names: []string{"state", "province", "region", "region", "land"}, Types: textTypes, Mask: "'TS'"},
	{Category: "Zip Code", Names: []string{"zip", "postal", "postcode", "zipcode", "psc", "plz"}, Types: textTypes, Mask: "LPAD(({pki} % 100000)::text, 5, '0')"},
	{Category: "Country", Names: []string{"country", "countrycode", "zeme", "land"}, Types: textTypes, Mask: "'US'"},
	{Category: "Latitude", Names: []string{"lat", "latitude"}, Types: numericTypes, Mask: "37.0 + ({pki} % 100) * 0.01"},
	{Category: "Longitude", Names: []string{"lon", "lng", "longitude"}, Types: numericTypes, Mask: "-122.0 + ({pki} % 100) * 0.01"},

	// Financial
	{Category: "Credit Card", Names: []string{"creditcard", "ccnumber", "cardnumber", "pan"}, Types: textTypes, Mask: "'4532000000' || LPAD(({pki} % 10000)::text, 4, '0')"},
	{Category: "CVV", Names: []string{"cvv", "cvc", "cardcode"}, Types: textTypes, Mask: "LPAD(({pki} % 1000)::text, 3, '0')"},
	{Category: "Bank Account", Names: []string{"accountnumber", "bankaccount", "iban", "kontonummer", "kontonr", "cislouctu"}, Types: textTypes, Mask: "'XXXX' || LPAD(({pki} % 10000)::text, 4, '0')"},
	{Category: "Routing Number", Names: []string{"routing", "routingnumber", "sortcode"}, Types: textTypes, Mask: "'021000021'"},
	{Category: "Salary", Names: []string{"salary", "compensation", "wage", "income", "gehalt", "plat"}, Types: numericTypes, Mask: "50000 + ({pki} % 100) * 1000"},

	// Secrets / API keys
	{Category: "API Key", Names: []string{"apikey", "apisecret", "secretkey", "accesskey", "clientsecret", "privatekey", "signingkey", "encryptionkey", "webhooksecret", "appsecret", "appkey"}, Types: textTypes, Mask: "'sk_test_' || LPAD({pki}::text, 24, '0')"},
	{Category: "Access Token", Names: []string{"token", "accesstoken", "authtoken", "apitoken", "refreshtoken", "bearertoken"}, Types: textTypes, Mask: "'tok_test_' || LPAD({pki}::text, 24, '0')"},
}

var noiseSuffixes = map[string]bool{
	"type": true, "template": true, "subject": true, "body": true,
	"format": true, "policy": true, "hint": true, "log": true,
	"error": true, "config": true, "url": true, "prefix": true,
	"method": true, "provider": true, "source": true, "reason": true,
	"description": true, "mode": true, "level": true, "regex": true,
	"pattern": true, "rule": true, "setting": true, "option": true,
	"preference": true,
}

func hasNoiseSuffix(colName string) bool {
	words := strings.Split(strings.ToLower(colName), "_")
	if len(words) <= 1 {
		return false
	}
	return noiseSuffixes[words[len(words)-1]]
}

// word level matching (user_email is going to be processed like [user,email])
func matchesColumnName(colName string, patterns []string) bool {
	lower := strings.ToLower(colName)
	words := strings.Split(lower, "_")
	for _, p := range patterns {
		if slices.Contains(words, p) {
			return true
		}
		if strings.ReplaceAll(lower, "_", "") == p {
			return true
		}
	}
	return false
}

func matchesType(colType string, typePatterns []string) bool {
	lower := strings.ToLower(colType)
	for _, t := range typePatterns {
		if strings.HasPrefix(lower, t) {
			return true
		}
	}
	return false
}

func isTextType(colType string) bool {
	return matchesType(colType, textTypes)
}

func isIntegerType(colType string) bool {
	lower := strings.ToLower(colType)
	return strings.HasPrefix(lower, "integer") ||
		strings.HasPrefix(lower, "bigint") ||
		strings.HasPrefix(lower, "smallint")
}
