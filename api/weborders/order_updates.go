package weborders

import (
	"regexp"
	"strings"
)

// Prefill from ordering web: "order_updates: web-TGFY-W-XXXXXX"
var orderUpdatesRe = regexp.MustCompile(`(?i)order_updates:\s*(?:web-)?([A-Za-z0-9_-]+)`)

// ParseOrderUpdatesRef extracts the order ref from an inbound WhatsApp body.
// Accepts "order_updates: web-TGFY-W-ABC" or "order_updates: TGFY-W-ABC".
func ParseOrderUpdatesRef(text string) string {
	m := orderUpdatesRe.FindStringSubmatch(strings.TrimSpace(text))
	if len(m) < 2 {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(m[1]))
}

func OrderDetailsLinkMessage(publicBase, orderRef string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if base == "" {
		base = "https://order.tangify.in"
	}
	ref := strings.TrimSpace(orderRef)
	return "Please click on this link - " + base + "/orders/" + ref
}
