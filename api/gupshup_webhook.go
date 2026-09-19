package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"tangify-backend-lambda/loyalty"
	"tangify-backend-lambda/users"
	"tangify-backend-lambda/weborders"
)

var loyaltySessionIDRe = regexp.MustCompile(`(?i)order:\s*(\S+)`)

type LoyaltyWaLinkPayload struct {
	SessionID     string `json:"sessionId"`
	Phone         string `json:"phone"`
	PointsBalance int64  `json:"pointsBalance"`
}

// parseGupshupInboundText extracts sender phone and message body from inbound webhooks.
// Primary format: Gupshup v3 passthrough (Meta Cloud API — entry/changes/value/messages).
// Fallback: legacy Gupshup v2 flat payload (mobile/text) for older subscriptions.
func parseGupshupInboundText(body string) (sender, text string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return "", ""
	}
	if sender, text = parseMetaV3Inbound(root); sender != "" && text != "" {
		return sender, text
	}
	return extractLegacyGupshupSender(root), extractLegacyGupshupText(root)
}

func parseMetaV3Inbound(root map[string]any) (sender, text string) {
	entries, ok := root["entry"].([]any)
	if !ok {
		return "", ""
	}
	for _, entryRaw := range entries {
		entry, ok := entryRaw.(map[string]any)
		if !ok {
			continue
		}
		changes, ok := entry["changes"].([]any)
		if !ok {
			continue
		}
		for _, changeRaw := range changes {
			change, ok := changeRaw.(map[string]any)
			if !ok {
				continue
			}
			field := strings.TrimSpace(asString(change["field"]))
			if field != "" && field != "messages" {
				continue
			}
			value, ok := change["value"].(map[string]any)
			if !ok {
				continue
			}
			messages, ok := value["messages"].([]any)
			if !ok {
				continue
			}
			for _, msgRaw := range messages {
				msg, ok := msgRaw.(map[string]any)
				if !ok {
					continue
				}
				from := strings.TrimSpace(asString(msg["from"]))
				msgText := extractMetaV3MessageText(msg)
				if from != "" && msgText != "" {
					return from, msgText
				}
			}
		}
	}
	return "", ""
}

func extractMetaV3MessageText(msg map[string]any) string {
	msgType := strings.TrimSpace(asString(msg["type"]))
	if msgType != "" && msgType != "text" {
		return ""
	}
	textObj, ok := msg["text"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(textObj["body"]))
}

func extractLegacyGupshupSender(root map[string]any) string {
	for _, key := range []string{"mobile", "source", "sender", "from"} {
		if v := strings.TrimSpace(asString(root[key])); v != "" {
			return v
		}
	}
	if payload, ok := root["payload"].(map[string]any); ok {
		for _, key := range []string{"source", "sender", "from"} {
			if v := strings.TrimSpace(asString(payload[key])); v != "" {
				return v
			}
		}
	}
	return ""
}

func extractLegacyGupshupText(root map[string]any) string {
	for _, key := range []string{"text", "message", "body"} {
		if v := strings.TrimSpace(asString(root[key])); v != "" {
			return v
		}
	}
	if payload, ok := root["payload"].(map[string]any); ok {
		if t := strings.TrimSpace(asString(payload["text"])); t != "" {
			return t
		}
		if inner, ok := payload["payload"].(map[string]any); ok {
			if t := strings.TrimSpace(asString(inner["text"])); t != "" {
				return t
			}
		}
	}
	return ""
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return ""
	}
}

func parseLoyaltySessionID(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	m := loyaltySessionIDRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// isPointsBalanceQuery is true for short balance check messages like "points".
func isPointsBalanceQuery(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.Join(strings.Fields(t), " ")
	t = strings.Trim(t, "!.?।")
	switch t {
	case "points", "point", "my points", "points balance", "balance":
		return true
	default:
		return false
	}
}

func handleGupshupInbound(
	ctx context.Context,
	body string,
	wallet *loyalty.WalletProvider,
	webLogin *weborders.LoginService,
	now int64,
) error {
	sender, text := parseGupshupInboundText(body)
	if sender == "" || text == "" {
		log.Println("gupshup inbound: ignored (no sender or text)")
		return nil
	}
	if weborders.IsWebLoginText(text) {
		return handleWebLoginInbound(ctx, sender, webLogin, now)
	}
	if ref := weborders.ParseOrderUpdatesRef(text); ref != "" {
		return handleOrderUpdatesInbound(ctx, sender, ref)
	}
	sessionID := parseLoyaltySessionID(text)
	if sessionID == "" {
		if isPointsBalanceQuery(text) {
			return handlePointsBalanceInbound(ctx, sender, wallet, now)
		}
		log.Println("gupshup inbound: ignored (no session id in text)")
		return nil
	}
	canon, err := users.CanonicalPhone(sender)
	if err != nil {
		log.Println("gupshup inbound: invalid phone:", err)
		return nil
	}
	view, err := wallet.GetOrCreateByPhone(ctx, canon, now)
	if err != nil {
		return err
	}
	phone10 := canon
	if len(phone10) == 12 && strings.HasPrefix(phone10, "91") {
		phone10 = phone10[2:]
	}
	payload := LoyaltyWaLinkPayload{
		SessionID:     sessionID,
		Phone:         phone10,
		PointsBalance: view.PointsBalance,
	}
	if err := sendLoyaltyBalanceReply(ctx, sender, view.PointsBalance); err != nil {
		log.Println("gupshup inbound: balance reply:", err)
	}
	ably := ablyPublisher()
	if ably == nil || !ably.enabled {
		log.Println("gupshup inbound: Ably disabled, skip publish")
		return nil
	}
	if err := ably.PublishLoyaltyWaLink(ctx, payload); err != nil {
		return err
	}
	log.Printf("gupshup inbound: loyalty:wa-link session=%s phone=%s balance=%d", sessionID, phone10, view.PointsBalance)
	return nil
}

func handlePointsBalanceInbound(
	ctx context.Context,
	sender string,
	wallet *loyalty.WalletProvider,
	now int64,
) error {
	if wallet == nil {
		log.Println("gupshup inbound: points query ignored (wallet nil)")
		return nil
	}
	canon, err := users.CanonicalPhone(sender)
	if err != nil {
		log.Println("gupshup inbound: points query invalid phone:", err)
		return nil
	}
	view, err := wallet.GetOrCreateByPhone(ctx, canon, now)
	if err != nil {
		return err
	}
	if err := sendLoyaltyBalanceReply(ctx, sender, view.PointsBalance); err != nil {
		return err
	}
	log.Printf("gupshup inbound: points balance phone=%s balance=%d", canon, view.PointsBalance)
	return nil
}

func handleWebLoginInbound(ctx context.Context, sender string, webLogin *weborders.LoginService, now int64) error {
	if webLogin == nil {
		log.Println("gupshup inbound: web login ignored (service nil)")
		return nil
	}
	nonce, err := webLogin.StartFromWhatsApp(ctx, sender, now)
	if err != nil {
		return err
	}
	msg := weborders.ContinueLinkMessage(weborders.OrderPublicBaseURL(), nonce)
	if err := sendGupshupTextMessage(ctx, sender, msg); err != nil {
		return err
	}
	log.Printf("gupshup inbound: web login nonce=%s", nonce)
	return nil
}

func handleOrderUpdatesInbound(ctx context.Context, sender, orderRef string) error {
	msg := weborders.OrderDetailsLinkMessage(weborders.OrderPublicBaseURL(), orderRef)
	if err := sendGupshupTextMessage(ctx, sender, msg); err != nil {
		return err
	}
	log.Printf("gupshup inbound: order updates link ref=%s", orderRef)
	return nil
}

func loyaltyBalanceReplyText(balance int64) string {
	return fmt.Sprintf("Tangify: Your points balance is %d", balance)
}

func sendLoyaltyBalanceReply(ctx context.Context, destination string, balance int64) error {
	dest := digitsOnly(destination)
	if dest == "" {
		return fmt.Errorf("destination phone required")
	}
	return sendGupshupTextMessage(ctx, dest, loyaltyBalanceReplyText(balance))
}
