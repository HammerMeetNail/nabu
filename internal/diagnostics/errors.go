// Package diagnostics classifies failures without rendering errors, which can
// contain SQL values, email addresses or capability URLs.
package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"regexp"
)

var sqlState = regexp.MustCompile(`^[0-9A-Z]{5}$`)

func ErrorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var sql interface{ SQLState() string }
	if errors.As(err, &sql) && sqlState.MatchString(sql.SQLState()) {
		return "sql_" + sql.SQLState()
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return "network_timeout"
		}
		return "network"
	}
	return "internal"
}

func RequestID() string {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(id[:])
}
