// Package model holds the domain entities of the aitrack server.
// These structs carry no persistence-framework coupling; the JSON tags
// present on API-facing entities describe the wire contract (protocol v1.2)
// and are part of the domain's published contract, not a transport adapter detail.
package model

import "time"

// Token is an issued API credential record.
type Token struct {
	ID         int64
	TokenHash  string
	TokenKey   string
	HmacSecret string // encrypted at rest, decrypted in memory for callers
	Owner      string
	Note       string
	Active     bool
	CreatedAt  time.Time
}
