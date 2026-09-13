package business

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"order-service/internal/services/order/models"
)

func TestCursorRoundTripAndDomainSeparation(t *testing.T) {
	key := strings.Repeat("k", 32)
	codec := newCursorCodec(key)
	user := uuid.New()
	position := models.OrderPosition{CreatedAt: time.Date(2026, 9, 13, 1, 2, 3, 123456000, time.UTC), OrderID: uuid.New()}
	token, err := codec.encode(user, position)
	if err != nil {
		t.Fatal(err)
	}
	got, err := codec.decode(token, user)
	if err != nil || got.OrderID != position.OrderID || !got.CreatedAt.Equal(position.CreatedAt) {
		t.Fatalf("cursor lost exact position: %v, %v", got, err)
	}
	parts := strings.Split(token, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
	sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
	raw := hmac.New(sha256.New, []byte(key))
	raw.Write(payload)
	if hmac.Equal(raw.Sum(nil), sig) {
		t.Fatal("cursor did not separate its signing key from service authentication")
	}
	if _, err := newCursorCodec(strings.Repeat("z", 32)).decode(token, user); err == nil {
		t.Fatal("key rotation accepted old cursor")
	}
}

func TestCursorRejectsInvalidTokens(t *testing.T) {
	codec := newCursorCodec(strings.Repeat("k", 32))
	user := uuid.New()
	payload := cursorPayload{1, user.String(), "2026-09-13T01:02:03.123456Z", uuid.NewString()}
	sign := func(p cursorPayload) string {
		b, _ := json.Marshal(p)
		return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(codec.sign(b))
	}
	valid := sign(payload)
	for _, token := range []string{"", "garbage", ".", valid + ".x", strings.Repeat("x", MaxCursorSize+1), valid + "\n", "\n" + valid, valid + "=", valid[:len(valid)-4] + "abcd"} {
		if _, err := codec.decode(token, user); err == nil {
			t.Error("accepted malformed or tampered cursor")
		}
	}
	if _, err := codec.decode(valid, uuid.New()); err == nil {
		t.Fatal("accepted another owner's cursor")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*cursorPayload)
	}{
		{"version", func(p *cursorPayload) { p.Version = 2 }},
		{"missing_version", func(p *cursorPayload) { p.Version = 0 }},
		{"timestamp", func(p *cursorPayload) { p.CreatedAt = "not-a-date" }},
		{"nanosecond", func(p *cursorPayload) { p.CreatedAt = "2026-09-13T01:02:03.123456789Z" }},
		{"offset", func(p *cursorPayload) { p.CreatedAt = "2026-09-13T01:02:03.123456+00:00" }},
		{"order_uuid", func(p *cursorPayload) { p.OrderID = "bad" }},
		{"nil_order", func(p *cursorPayload) { p.OrderID = uuid.Nil.String() }},
		{"owner", func(p *cursorPayload) { p.UserID = uuid.NewString() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := payload
			tc.mutate(&p)
			if _, err := codec.decode(sign(p), user); err == nil {
				t.Fatal("accepted invalid signed cursor payload")
			}
		})
	}
}
