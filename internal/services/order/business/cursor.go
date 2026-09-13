package business

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"order-service/internal/services/order/models"

	"github.com/google/uuid"
)

const MaxCursorSize = 1024
const cursorDomain = "swaad/order-service/GetUserOrders/cursor/v1"

var ErrInvalidCursor = errors.New("invalid order cursor")

type cursorPayload struct {
	Version   int    `json:"v"`
	UserID    string `json:"u"`
	CreatedAt string `json:"t"`
	OrderID   string `json:"o"`
}

type cursorCodec struct{ key []byte }

func newCursorCodec(serviceKey string) cursorCodec {
	derive := hmac.New(sha256.New, []byte(serviceKey))
	derive.Write([]byte(cursorDomain))
	return cursorCodec{key: derive.Sum(nil)}
}

func (c cursorCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func (c cursorCodec) encode(user uuid.UUID, position models.OrderPosition) (string, error) {
	if user == uuid.Nil || position.OrderID == uuid.Nil || position.CreatedAt.IsZero() || position.CreatedAt.Nanosecond()%1000 != 0 {
		return "", ErrInvalidCursor
	}
	payload, err := json.Marshal(cursorPayload{1, user.String(), position.CreatedAt.UTC().Format(time.RFC3339Nano), position.OrderID.String()})
	if err != nil {
		return "", ErrInvalidCursor
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(c.sign(payload)), nil
}

func (c cursorCodec) decode(token string, user uuid.UUID) (*models.OrderPosition, error) {
	if len(token) == 0 || len(token) > MaxCursorSize {
		return nil, ErrInvalidCursor
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidCursor
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, c.sign(payload)) {
		return nil, ErrInvalidCursor
	}
	if base64.RawURLEncoding.EncodeToString(payload) != parts[0] || base64.RawURLEncoding.EncodeToString(sig) != parts[1] {
		return nil, ErrInvalidCursor
	}
	var decoded cursorPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, ErrInvalidCursor
	}
	canonical, _ := json.Marshal(decoded)
	if !bytes.Equal(payload, canonical) || decoded.Version != 1 || user == uuid.Nil || decoded.UserID != user.String() {
		return nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(decoded.OrderID)
	if err != nil || id == uuid.Nil || id.String() != decoded.OrderID {
		return nil, ErrInvalidCursor
	}
	created, err := time.Parse(time.RFC3339Nano, decoded.CreatedAt)
	if err != nil || created.IsZero() || created.Nanosecond()%1000 != 0 || created.UTC().Format(time.RFC3339Nano) != decoded.CreatedAt {
		return nil, ErrInvalidCursor
	}
	return &models.OrderPosition{OrderID: id, CreatedAt: created}, nil
}
