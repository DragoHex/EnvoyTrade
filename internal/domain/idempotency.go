package domain

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// idempotencyTagLen is the truncated hex-digest length used for the tag.
// Kite's order `tag` field caps at 20 characters, so this is the hard
// ceiling. Hex keeps every character alphanumeric, which is what the
// broker's tag field accepts.
const idempotencyTagLen = 20

// IdempotencyTag deterministically derives a short tag from a master fill
// and a follower account. It is written to follower_orders before the
// broker call and sent as the order's own `tag`, so a duplicated dispatch
// (WS redelivery, retried insert) collides on a unique index instead of
// placing a second live order.
func IdempotencyTag(masterFillID int64, followerID uuid.UUID) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d:%s", masterFillID, followerID))
	return fmt.Sprintf("%x", sum)[:idempotencyTagLen]
}
