// Package callback turns Zerodha Kite's two order-update delivery
// mechanisms — the postback HTTP webhook and the WS ticker's
// OnOrderUpdate — into the domain events the rest of EnvoyTrade already
// understands (domain.MasterFill, domain.OrderUpdate), queuing them so a
// slow DB/engine call never blocks the HTTP handler or the WS read loop
// (PLAN.md §3.1, §3.4).
package callback

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// VerifyChecksum checks a Kite postback's checksum field: sha256(order_id
// + order_timestamp + api_secret), hex-encoded (kite.trade/docs/connect/v3/postbacks).
// apiSecret must be the secret of the account the order belongs to — every
// account has its own Kite Connect app, so the caller must resolve which
// account before calling this.
func VerifyChecksum(apiSecret, orderID, orderTimestamp, checksum string) bool {
	sum := sha256.Sum256([]byte(orderID + orderTimestamp + apiSecret))
	want := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(checksum)) == 1
}
