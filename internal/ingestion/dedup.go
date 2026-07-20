package ingestion

import "crypto/sha256"

// Dedup tracks seen chunks by content hash to prevent duplicates
// across ingestion runs.
type Dedup struct {
	seen map[[32]byte]struct{}
}

func NewDedup() *Dedup {
	return &Dedup{seen: make(map[[32]byte]struct{})}
}

// Seen returns true if the content has already been ingested.
// Returns false on first call for a given content, then true for subsequent calls.
func (d *Dedup) Seen(content string) bool {
	hash := sha256.Sum256([]byte(content))
	if _, exists := d.seen[hash]; exists {
		return true
	}
	d.seen[hash] = struct{}{}
	return false
}
