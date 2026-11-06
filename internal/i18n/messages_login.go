package i18n

// Login strings live in their own catalogs because the sign-in page has no
// user settings to read; it negotiates a locale from Accept-Language.
var loginEnglish = map[string]string{
	"login.title":              "Gofer - Sign In",
	"login.tagline":            "A self-hosted email client",
	"login.username":           "Username",
	"login.password":           "Password",
	"login.forgot_password":    "Forgot password?",
	"login.continue":           "Continue",
	"login.sign_in":            "Sign in",
	"login.passkey_fallback":   "Use your password if the passkey is unavailable.",
	"login.passkey_sign_in":    "Sign in with a passkey",
	"login.passkey_noscript":   "JavaScript is required for passkey sign-in. You can still use your password.",
	"login.or":                 "or",
	"login.continue_google":    "Continue with Google",
	"login.continue_microsoft": "Continue with Microsoft",
	"login.continue_oidc":      "Continue with %s",
	"login.recovery":           "Lost access? Use the local Gofer recovery command to reset your password.",
	"login.invitation":         "Have an invitation?",
	"login.setup_account":      "Set up your account",
}

var loginFrench = map[string]string{
	"login.title":              "Gofer - Connexion",
	"login.tagline":            "Un client de messagerie auto-hébergé",
	"login.username":           "Nom d'utilisateur",
	"login.password":           "Mot de passe",
	"login.forgot_password":    "Mot de passe oublié ?",
	"login.continue":           "Continuer",
	"login.sign_in":            "Se connecter",
	"login.passkey_fallback":   "Utilisez votre mot de passe si la clé d'accès n'est pas disponible.",
	"login.passkey_sign_in":    "Se connecter avec une clé d'accès",
	"login.passkey_noscript":   "JavaScript est nécessaire pour la connexion par clé d'accès. Vous pouvez toujours utiliser votre mot de passe.",
	"login.or":                 "ou",
	"login.continue_google":    "Continuer avec Google",
	"login.continue_microsoft": "Continuer avec Microsoft",
	"login.continue_oidc":      "Continuer avec %s",
	"login.recovery":           "Accès perdu ? Utilisez la commande de récupération locale de Gofer pour réinitialiser votre mot de passe.",
	"login.invitation":         "Vous avez une invitation ?",
	"login.setup_account":      "Configurer votre compte",
}

func init() {
	for key, value := range loginEnglish {
		messages[English][key] = value
	}
	for key, value := range loginFrench {
		messages[French][key] = value
	}
}
