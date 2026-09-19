package weborders

import (
	"crypto/rand"
	"regexp"
	"strings"
)

const TableNameLoginNonce = "tangify-login-nonce"

const (
	loginNonceLength     = 7
	loginNonceTTLSeconds = 10 * 60
	loginNoncePutRetries = 8
)

// Crockford base32 without I, L, O, U — readable in a WhatsApp URL.
const nonceAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var webLoginRe = regexp.MustCompile(`(?i)login:\s*web\b`)

type LoginNonce struct {
	Nonce  string `json:"nonce"`
	UserID string `json:"user_id"`
	JWT    string `json:"jwt"`
	Phone  string `json:"phone"`
	TTL    int64  `json:"ttl"`
}

type ContinueResponse struct {
	UserID string `json:"user_id"`
	JWT    string `json:"jwt"`
	Phone  string `json:"phone"`
}

type ContinueRequest struct {
	Key   string `json:"key"`
	Nonce string `json:"nonce"`
}

func (r ContinueRequest) Code() string {
	code := strings.TrimSpace(r.Key)
	if code == "" {
		code = strings.TrimSpace(r.Nonce)
	}
	return strings.ToUpper(code)
}

// IsWebLoginText is true for the ordering-web prefill "login: web".
func IsWebLoginText(text string) bool {
	return webLoginRe.MatchString(strings.TrimSpace(text))
}

func generateNonce() (string, error) {
	buf := make([]byte, loginNonceLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, loginNonceLength)
	alpha := []byte(nonceAlphabet)
	for i, b := range buf {
		out[i] = alpha[int(b)%len(alpha)]
	}
	return string(out), nil
}

func displayPhone(canon string) string {
	if len(canon) == 12 && strings.HasPrefix(canon, "91") {
		return canon[2:]
	}
	return canon
}

func ContinueLinkMessage(publicBase, nonce string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if base == "" {
		base = "https://order.tangify.in"
	}
	return "Please click on this link - " + base + "/continue/" + nonce
}

func OrderPublicBaseURL() string {
	base := strings.TrimSpace(envTrim("WEB_ORDER_PUBLIC_BASE_URL"))
	if base == "" {
		return "https://order.tangify.in"
	}
	return strings.TrimRight(base, "/")
}
