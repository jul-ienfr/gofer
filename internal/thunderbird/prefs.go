package thunderbird

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const MaxPrefsBytes = 16 << 20

var (
	ErrNotPrefs   = errors.New("this file is not a Thunderbird prefs.js file")
	ErrTooLarge   = errors.New("the prefs.js file is too large")
	ErrNoAccounts = errors.New("no mail accounts were found in this file")
)

type Profile struct {
	Accounts []Account
	Warnings []Warning
}

type Warning struct {
	Account string
	Message string
}

type Account struct {
	Ref        string
	Server     Server
	Identities []Identity
}

type Server struct {
	Type string
	Host string
	// SocketType is -1 when Thunderbird has no value for it.
	SocketType int
	Port       int
	Username   string
}

type Identity struct {
	Ref     string
	Email   string
	Name    string
	ReplyTo string
	SMTP    *SMTPServer

	Signature          string
	SignatureIsHTML    bool
	SignatureOnNew     bool
	SignatureOnReply   bool
	SignatureOnForward bool
	SignatureBelow     bool
	SignatureKnown     bool
}

type SMTPServer struct {
	Ref  string
	Host string
	// SocketType is -1 when Thunderbird has no value for it.
	SocketType int
	Port       int
	Username   string
}

func Parse(data []byte) (*Profile, error) {
	if len(data) > MaxPrefsBytes {
		return nil, ErrTooLarge
	}
	prefs, err := parsePrefs(data)
	if err != nil {
		return nil, err
	}
	return buildProfile(prefs)
}

func (p *Profile) AccountEmail(account Account) string {
	for _, identity := range account.Identities {
		if identity.Email != "" {
			return identity.Email
		}
	}
	return ""
}

type prefValue interface{}

type prefsParser struct {
	data []byte
	i    int
	line int
}

func parsePrefs(data []byte) (map[string]prefValue, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	p := &prefsParser{data: data, line: 1}
	prefs := map[string]prefValue{}
	for {
		p.skipSpace()
		if p.i >= len(p.data) {
			return prefs, nil
		}
		start := p.i
		name := p.readIdent()
		if name == "" {
			return nil, p.notPrefs("unexpected character")
		}
		p.skipSpace()
		if name != "user_pref" && name != "sticky_pref" {
			p.i = start
			p.skipStatement()
			continue
		}
		if p.i >= len(p.data) || p.data[p.i] != '(' {
			p.i = start
			p.skipStatement()
			continue
		}
		p.i++
		p.skipSpace()
		key, err := p.readString()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if !p.consume(',') {
			return nil, p.notPrefs("expected a comma after the preference name")
		}
		p.skipSpace()
		value, err := p.readValue()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.consume(',') {
			p.skipSpace()
		}
		if !p.consume(')') {
			return nil, p.notPrefs("expected the end of the preference")
		}
		if p.i < len(p.data) && p.data[p.i] == ';' {
			p.i++
		}
		prefs[key] = value
	}
}

func (p *prefsParser) readValue() (prefValue, error) {
	if p.i >= len(p.data) {
		return nil, p.notPrefs("expected a value")
	}
	switch c := p.data[p.i]; {
	case c == '"':
		return p.readString()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.readNumber()
	case c == 't' || c == 'f' || c == 'n':
		switch p.readIdent() {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		}
	}
	return nil, p.notPrefs("expected a value")
}

func (p *prefsParser) readNumber() (prefValue, error) {
	start := p.i
	if p.data[p.i] == '-' {
		p.i++
	}
	for p.i < len(p.data) {
		c := p.data[p.i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			p.i++
			continue
		}
		break
	}
	text := string(p.data[start:p.i])
	if strings.ContainsAny(text, ".eE") {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, p.notPrefs("invalid number")
		}
		return value, nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, p.notPrefs("invalid number")
	}
	return value, nil
}

func (p *prefsParser) readString() (string, error) {
	if p.i >= len(p.data) || p.data[p.i] != '"' {
		return "", p.notPrefs("expected a quoted string")
	}
	p.i++
	var out strings.Builder
	for p.i < len(p.data) {
		switch c := p.data[p.i]; c {
		case '"':
			p.i++
			return out.String(), nil
		case '\\':
			p.i++
			if p.i >= len(p.data) {
				return "", p.notPrefs("unfinished escape sequence")
			}
			escape := p.data[p.i]
			p.i++
			switch escape {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case 'b':
				out.WriteByte('\b')
			case 'f':
				out.WriteByte('\f')
			case 'v':
				out.WriteByte('\v')
			case '0':
				out.WriteByte(0)
			case '\n':
				p.line++
			case '\r':
				if p.i < len(p.data) && p.data[p.i] == '\n' {
					p.i++
				}
				p.line++
			case 'u':
				r, err := p.readHexRune(4)
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(r) && p.i+1 < len(p.data) && p.data[p.i] == '\\' && p.data[p.i+1] == 'u' {
					save := p.i
					p.i += 2
					if low, err := p.readHexRune(4); err == nil {
						if combined := utf16.DecodeRune(r, low); combined != utf8.RuneError {
							out.WriteRune(combined)
							continue
						}
					}
					p.i = save
					out.WriteRune(utf8.RuneError)
					continue
				}
				out.WriteRune(r)
			case 'x':
				r, err := p.readHexRune(2)
				if err != nil {
					return "", err
				}
				out.WriteRune(r)
			default:
				out.WriteByte(escape)
			}
		case '\n':
			return "", p.notPrefs("a string runs past the end of its line")
		default:
			out.WriteByte(c)
			p.i++
		}
	}
	return "", p.notPrefs("unfinished string")
}

func (p *prefsParser) readHexRune(size int) (rune, error) {
	value := 0
	for i := 0; i < size; i++ {
		if p.i >= len(p.data) {
			return 0, p.notPrefs("unfinished escape sequence")
		}
		digit := hexDigit(p.data[p.i])
		if digit < 0 {
			return 0, p.notPrefs("invalid hexadecimal escape")
		}
		value = value*16 + digit
		p.i++
	}
	return rune(value), nil
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (p *prefsParser) readIdent() string {
	start := p.i
	for p.i < len(p.data) {
		c := p.data[p.i]
		if c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (p.i > start && c >= '0' && c <= '9') {
			p.i++
			continue
		}
		break
	}
	return string(p.data[start:p.i])
}

func (p *prefsParser) skipSpace() {
	for p.i < len(p.data) {
		switch p.data[p.i] {
		case ' ', '\t', '\r', '\f':
			p.i++
		case '\n':
			p.line++
			p.i++
		case '/':
			if p.i+1 >= len(p.data) {
				return
			}
			switch p.data[p.i+1] {
			case '/':
				p.i += 2
				for p.i < len(p.data) && p.data[p.i] != '\n' {
					p.i++
				}
			case '*':
				p.i += 2
				for p.i+1 < len(p.data) && !(p.data[p.i] == '*' && p.data[p.i+1] == '/') {
					if p.data[p.i] == '\n' {
						p.line++
					}
					p.i++
				}
				if p.i+1 < len(p.data) {
					p.i += 2
				} else {
					p.i = len(p.data)
				}
			default:
				return
			}
		default:
			return
		}
	}
}

func (p *prefsParser) skipStatement() {
	for p.i < len(p.data) {
		switch c := p.data[p.i]; {
		case c == '"':
			p.skipQuoted()
		case c == '/' && p.i+1 < len(p.data) && p.data[p.i+1] == '/':
			for p.i < len(p.data) && p.data[p.i] != '\n' {
				p.i++
			}
		case c == '/' && p.i+1 < len(p.data) && p.data[p.i+1] == '*':
			p.i += 2
			for p.i+1 < len(p.data) && !(p.data[p.i] == '*' && p.data[p.i+1] == '/') {
				if p.data[p.i] == '\n' {
					p.line++
				}
				p.i++
			}
			if p.i+1 < len(p.data) {
				p.i += 2
			} else {
				p.i = len(p.data)
			}
		case c == ';':
			p.i++
			return
		default:
			if c == '\n' {
				p.line++
			}
			p.i++
		}
	}
}

func (p *prefsParser) skipQuoted() {
	p.i++
	for p.i < len(p.data) {
		switch p.data[p.i] {
		case '\\':
			p.i++
			if p.i < len(p.data) {
				if p.data[p.i] == '\n' {
					p.line++
				}
				p.i++
			}
		case '"':
			p.i++
			return
		case '\n':
			p.line++
			p.i++
		default:
			p.i++
		}
	}
}

func (p *prefsParser) consume(c byte) bool {
	if p.i >= len(p.data) || p.data[p.i] != c {
		return false
	}
	p.i++
	return true
}

func (p *prefsParser) notPrefs(reason string) error {
	return fmt.Errorf("This file is not a Thunderbird prefs.js file (line %d: %s).", p.line, reason)
}

func buildProfile(prefs map[string]prefValue) (*Profile, error) {
	refs := splitList(stringPref(prefs, "mail.accountmanager.accounts"))
	if len(refs) == 0 {
		refs = accountRefs(prefs)
	}
	if len(refs) == 0 {
		if !hasPrefPrefix(prefs, "mail.") {
			return nil, fmt.Errorf("%w.", ErrNotPrefs)
		}
		return nil, ErrNoAccounts
	}
	smtpServers := buildSMTPServers(prefs)
	profile := &Profile{}
	for _, ref := range refs {
		account, warnings := buildAccount(prefs, ref, smtpServers)
		profile.Warnings = append(profile.Warnings, warnings...)
		if account != nil {
			profile.Accounts = append(profile.Accounts, *account)
		}
	}
	if len(profile.Accounts) == 0 && len(profile.Warnings) == 0 {
		return nil, ErrNoAccounts
	}
	return profile, nil
}

func buildAccount(prefs map[string]prefValue, ref string, smtpServers map[string]SMTPServer) (*Account, []Warning) {
	serverRef := stringPref(prefs, "mail.account."+ref+".server")
	if serverRef == "" {
		return nil, []Warning{{Message: "An account in the file has no server settings and was skipped."}}
	}
	server := buildServer(prefs, serverRef)
	if strings.EqualFold(server.Type, "none") {
		return nil, nil
	}
	var identities []Identity
	for _, identityRef := range splitList(stringPref(prefs, "mail.account."+ref+".identities")) {
		identity := buildIdentity(prefs, identityRef, smtpServers)
		if identity.Email == "" {
			continue
		}
		identities = append(identities, identity)
	}
	if len(identities) == 0 {
		return nil, []Warning{{Message: "An account in the file has no email address and was skipped."}}
	}
	return &Account{Ref: ref, Server: server, Identities: identities}, nil
}

func buildServer(prefs map[string]prefValue, ref string) Server {
	prefix := "mail.server." + ref + "."
	server := Server{
		Type:       strings.ToLower(stringPref(prefs, prefix+"type")),
		Host:       stringPref(prefs, prefix+"hostname"),
		SocketType: -1,
		Port:       intPref(prefs, prefix+"port"),
		Username:   stringPref(prefs, prefix+"userName"),
	}
	if server.Username == "" {
		server.Username = stringPref(prefs, prefix+"username")
	}
	if socketType, ok := intPrefOK(prefs, prefix+"socketType"); ok {
		server.SocketType = socketType
	}
	return server
}

func buildIdentity(prefs map[string]prefValue, ref string, smtpServers map[string]SMTPServer) Identity {
	prefix := "mail.identity." + ref + "."
	identity := Identity{
		Ref:     ref,
		Email:   stringPref(prefs, prefix+"useremail"),
		Name:    stringPref(prefs, prefix+"fullName"),
		ReplyTo: stringPref(prefs, prefix+"reply_to"),
	}
	if smtpRef := stringPref(prefs, prefix+"smtpServer"); smtpRef != "" {
		if server, ok := smtpServers[smtpRef]; ok {
			identity.SMTP = &server
		}
	}
	identity.Signature = stringPref(prefs, prefix+"htmlSigText")
	if identity.Signature == "" {
		identity.Signature = stringPref(prefs, prefix+"sigText")
	}
	identity.SignatureKnown = identity.Signature != ""
	identity.SignatureIsHTML = boolPref(prefs, prefix+"htmlSigFormat", false)
	identity.SignatureOnNew = boolPref(prefs, prefix+"sig_on_new", true)
	identity.SignatureOnReply = boolPref(prefs, prefix+"sig_on_reply", true)
	identity.SignatureOnForward = boolPref(prefs, prefix+"sig_on_fwd", false)
	identity.SignatureBelow = boolPref(prefs, prefix+"sig_bottom", false)
	return identity
}

func buildSMTPServers(prefs map[string]prefValue) map[string]SMTPServer {
	refs := splitList(stringPref(prefs, "mail.smtpservers"))
	if len(refs) == 0 {
		seen := map[string]bool{}
		for key := range prefs {
			rest, ok := strings.CutPrefix(key, "mail.smtpserver.")
			if !ok {
				continue
			}
			ref, field, ok := strings.Cut(rest, ".")
			if !ok || field != "hostname" || seen[ref] {
				continue
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
		sort.Strings(refs)
	}
	servers := make(map[string]SMTPServer, len(refs))
	for _, ref := range refs {
		prefix := "mail.smtpserver." + ref + "."
		server := SMTPServer{
			Ref:        ref,
			Host:       stringPref(prefs, prefix+"hostname"),
			SocketType: -1,
			Port:       intPref(prefs, prefix+"port"),
			Username:   stringPref(prefs, prefix+"username"),
		}
		if server.Username == "" {
			server.Username = stringPref(prefs, prefix+"userName")
		}
		if socketType, ok := intPrefOK(prefs, prefix+"socketType"); ok {
			server.SocketType = socketType
		} else if socketType, ok := intPrefOK(prefs, prefix+"try_ssl"); ok {
			server.SocketType = socketType
		}
		servers[ref] = server
	}
	return servers
}

func accountRefs(prefs map[string]prefValue) []string {
	var refs []string
	for key := range prefs {
		rest, ok := strings.CutPrefix(key, "mail.account.")
		if !ok {
			continue
		}
		ref, field, ok := strings.Cut(rest, ".")
		if ok && field == "server" {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringPref(prefs map[string]prefValue, key string) string {
	value, _ := prefs[key].(string)
	return strings.TrimSpace(value)
}

func intPref(prefs map[string]prefValue, key string) int {
	value, _ := intPrefOK(prefs, key)
	return value
}

func boolPref(prefs map[string]prefValue, key string, fallback bool) bool {
	value, ok := prefs[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func intPrefOK(prefs map[string]prefValue, key string) (int, bool) {
	switch value := prefs[key].(type) {
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func hasPrefPrefix(prefs map[string]prefValue, prefix string) bool {
	for key := range prefs {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
