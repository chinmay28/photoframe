// Package library aggregates photos from all configured sources into one
// catalogue that the player draws from.
package library

import (
	"crypto/sha1"
	"encoding/hex"
	"time"
)

// Photo is one displayable image that has been made available locally.
type Photo struct {
	ID       string    `json:"id"`
	SourceID string    `json:"sourceId"`
	Path     string    `json:"-"` // local file; never exposed over the API
	TakenAt  time.Time `json:"takenAt"`
	Title    string    `json:"title,omitempty"`
	// Name is the photo's file name at its source, for people to recognise
	// it by, e.g. "IMG_1234.jpg".
	Name string `json:"name,omitempty"`
}

// PhotoID derives a stable, URL-safe photo ID from a source ID and a key
// that is unique within that source (a relative path, a remote asset ID...).
func PhotoID(sourceID, key string) string {
	sum := sha1.Sum([]byte(key))
	return sourceID + "-" + hex.EncodeToString(sum[:8])
}
