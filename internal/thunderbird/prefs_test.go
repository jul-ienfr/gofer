package thunderbird

import (
	"errors"
	"strings"
	"testing"
)

const samplePrefs = `// Mozilla User Preferences

user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1,id2");
user_pref("mail.account.account2.server", "server2");
user_pref("mail.account.account2.identities", "id3");
user_pref("mail.account.account3.server", "server3");
user_pref("mail.account.account3.identities", "id5");
user_pref("mail.account.account4.server", "server4");
user_pref("mail.account.account4.identities", "id4");
user_pref("mail.accountmanager.accounts", "account1,account2,account3,account4");
user_pref("mail.identity.id1.useremail", "robin@example.test");
user_pref("mail.identity.id1.fullName", "Robin \"The\" Example");
user_pref("mail.identity.id1.reply_to", "team@example.test");
user_pref("mail.identity.id1.smtpServer", "smtp1");
user_pref("mail.identity.id1.htmlSigText", "<p>Robin</p>");
user_pref("mail.identity.id1.htmlSigFormat", true);
user_pref("mail.identity.id1.sig_on_new", true);
user_pref("mail.identity.id1.sig_on_reply", true);
user_pref("mail.identity.id1.sig_on_fwd", false);
user_pref("mail.identity.id1.sig_bottom", true);
user_pref("mail.identity.id2.useremail", "sales@example.test");
user_pref("mail.identity.id2.fullName", "Robin Sales");
user_pref("mail.identity.id2.smtpServer", "smtp1");
user_pref("mail.identity.id2.htmlSigText", "Plain signature");
user_pref("mail.identity.id2.htmlSigFormat", false);
user_pref("mail.identity.id3.useremail", "old@example.test");
user_pref("mail.identity.id4.useremail", "elodie@example.org");
user_pref("mail.identity.id4.fullName", "\u00c9lodie \ud83d\ude00");
user_pref("mail.identity.id5.useremail", "local@example.test");
user_pref("mail.server.server1.type", "imap");
user_pref("mail.server.server1.hostname", "imap.example.test");
user_pref("mail.server.server1.port", 993);
user_pref("mail.server.server1.socketType", 2);
user_pref("mail.server.server1.userName", "robin");
user_pref("mail.server.server2.type", "pop3");
user_pref("mail.server.server2.hostname", "pop.example.test");
user_pref("mail.server.server2.port", 995);
user_pref("mail.server.server2.socketType", 2);
user_pref("mail.server.server2.userName", "old");
user_pref("mail.server.server3.type", "none");
user_pref("mail.server.server4.type", "imap");
user_pref("mail.server.server4.hostname", "mail.example.org");
user_pref("mail.smtpservers", "smtp1");
user_pref("mail.smtpserver.smtp1.hostname", "smtp.example.test");
user_pref("mail.smtpserver.smtp1.port", 587);
user_pref("mail.smtpserver.smtp1.socketType", 3);
user_pref("mail.smtpserver.smtp1.username", "robin-smtp");
`

func TestParseReadsAccountsIdentitiesAndServers(t *testing.T) {
	profile, err := Parse([]byte(samplePrefs))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(profile.Accounts) != 3 {
		t.Fatalf("accounts = %d, want 3", len(profile.Accounts))
	}
	account := profile.Accounts[0]
	if account.Ref != "account1" {
		t.Fatalf("ref = %q", account.Ref)
	}
	if account.Server.Type != "imap" || account.Server.Host != "imap.example.test" || account.Server.Port != 993 || account.Server.SocketType != 2 || account.Server.Username != "robin" {
		t.Fatalf("server = %+v", account.Server)
	}
	if len(account.Identities) != 2 {
		t.Fatalf("identities = %d", len(account.Identities))
	}
	primary := account.Identities[0]
	if primary.Email != "robin@example.test" || primary.Name != `Robin "The" Example` || primary.ReplyTo != "team@example.test" {
		t.Fatalf("primary identity = %+v", primary)
	}
	if primary.SMTP == nil || primary.SMTP.Host != "smtp.example.test" || primary.SMTP.Port != 587 || primary.SMTP.SocketType != 3 || primary.SMTP.Username != "robin-smtp" {
		t.Fatalf("smtp = %+v", primary.SMTP)
	}
	if primary.Signature != "<p>Robin</p>" || !primary.SignatureIsHTML || !primary.SignatureKnown {
		t.Fatalf("signature = %+v", primary)
	}
	if !primary.SignatureOnNew || !primary.SignatureOnReply || primary.SignatureOnForward || !primary.SignatureBelow {
		t.Fatalf("signature flags = %+v", primary)
	}
	alias := account.Identities[1]
	if alias.Signature != "Plain signature" || alias.SignatureIsHTML || !alias.SignatureKnown {
		t.Fatalf("alias signature = %+v", alias)
	}
	if got := profile.AccountEmail(account); got != "robin@example.test" {
		t.Fatalf("AccountEmail() = %q", got)
	}

	if account := profile.Accounts[1]; account.Server.Type != "pop3" || account.Identities[0].Email != "old@example.test" {
		t.Fatalf("second account = %+v", account)
	}
	last := profile.Accounts[2]
	if last.Server.SocketType != -1 || last.Server.Port != 0 || last.Server.Username != "" {
		t.Fatalf("unset server values = %+v", last.Server)
	}
	if last.Identities[0].Name != "Élodie 😀" {
		t.Fatalf("escaped name = %q", last.Identities[0].Name)
	}
	if last.Identities[0].SMTP != nil {
		t.Fatalf("identity without smtp server got %+v", last.Identities[0].SMTP)
	}
}

func TestParseSkipsLocalFoldersAndKeepsWarnings(t *testing.T) {
	profile, err := Parse([]byte(`
user_pref("mail.accountmanager.accounts", "account1,account2,account3");
user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1");
user_pref("mail.account.account2.server", "server2");
user_pref("mail.account.account2.identities", "id2");
user_pref("mail.account.account3.identities", "id3");
user_pref("mail.identity.id1.useremail", "a@example.test");
user_pref("mail.identity.id2.useremail", "b@example.test");
user_pref("mail.identity.id3.useremail", "c@example.test");
user_pref("mail.server.server1.type", "none");
user_pref("mail.server.server2.type", "imap");
user_pref("mail.server.server2.hostname", "imap.example.test");
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(profile.Accounts) != 1 || profile.Accounts[0].Ref != "account2" {
		t.Fatalf("accounts = %+v", profile.Accounts)
	}
	if len(profile.Warnings) != 1 || !strings.Contains(profile.Warnings[0].Message, "no server settings") {
		t.Fatalf("warnings = %+v", profile.Warnings)
	}
	if got := profile.AccountEmail(profile.Accounts[0]); got != "b@example.test" {
		t.Fatalf("AccountEmail() = %q", got)
	}
}

func TestParseAcceptsStickyPrefsCommentsAndContinuations(t *testing.T) {
	profile, err := Parse([]byte(`/* Mozilla prefs */
sticky_pref("mail.accountmanager.accounts", "account1"); // one account
user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1");
user_pref("mail.identity.id1.useremail", "continuation@example.test");
user_pref("mail.identity.id1.fullName", "Robin \
Sales");
user_pref("mail.server.server1.type", "imap");
user_pref("mail.server.server1.hostname", "imap.example.test");
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(profile.Accounts) != 1 || profile.Accounts[0].Identities[0].Name != "Robin Sales" {
		t.Fatalf("accounts = %+v", profile.Accounts)
	}
}

func TestParseLastValueWins(t *testing.T) {
	profile, err := Parse([]byte(`
user_pref("mail.accountmanager.accounts", "account1");
user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1");
user_pref("mail.identity.id1.useremail", "a@example.test");
user_pref("mail.server.server1.type", "imap");
user_pref("mail.server.server1.hostname", "old.example.test");
user_pref("mail.server.server1.hostname", "new.example.test");
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := profile.Accounts[0].Server.Host; got != "new.example.test" {
		t.Fatalf("host = %q", got)
	}
}

func TestParseFallsBackToTrySSL(t *testing.T) {
	profile, err := Parse([]byte(`
user_pref("mail.accountmanager.accounts", "account1");
user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1");
user_pref("mail.identity.id1.useremail", "a@example.test");
user_pref("mail.identity.id1.smtpServer", "smtp1");
user_pref("mail.server.server1.type", "imap");
user_pref("mail.server.server1.hostname", "imap.example.test");
user_pref("mail.smtpservers", "smtp1");
user_pref("mail.smtpserver.smtp1.hostname", "smtp.example.test");
user_pref("mail.smtpserver.smtp1.try_ssl", 3);
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := profile.Accounts[0].Identities[0].SMTP; got == nil || got.SocketType != 3 {
		t.Fatalf("smtp = %+v", got)
	}
}

func TestParseRejectsOtherFiles(t *testing.T) {
	for _, data := range []string{
		"var x = 1;",
		"",
		`user_pref("mail.smtpservers", "smtp1");`,
	} {
		_, err := Parse([]byte(data))
		if err == nil {
			t.Fatalf("Parse(%q) returned no error", data)
		}
	}
}

func TestParseRejectsBrokenStatement(t *testing.T) {
	if _, err := Parse([]byte(`user_pref("mail.accountmanager.accounts, "account1");`)); err == nil {
		t.Fatal("Parse() accepted a broken statement")
	}
}

func TestParseTooLarge(t *testing.T) {
	_, err := Parse(make([]byte, MaxPrefsBytes+1))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Parse() error = %v", err)
	}
}
