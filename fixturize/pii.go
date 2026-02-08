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
	{Category: "Username", Names: []string{"username", "loginname", "handle", "screenname"}, Types: textTypes, Mask: "'user' || {pk}"},
	{Category: "Password", Names: []string{"password", "passwd", "pwd", "passhash", "pwhash"}, Types: textTypes, Mask: "'$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWX'"},
	{Category: "Date of Birth", Names: []string{"dob", "birthdate", "dateofbirth", "birthday", "geburtsdatum", "date_naissance", "datum_narozeni"}, Types: []string{"date", "timestamp"}, Mask: "'1990-01-01'::date + ({pki} % 10000)"},
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

func isIntegerType(colType string) bool {
	lower := strings.ToLower(colType)
	return strings.HasPrefix(lower, "integer") ||
		strings.HasPrefix(lower, "bigint") ||
		strings.HasPrefix(lower, "smallint")
}
