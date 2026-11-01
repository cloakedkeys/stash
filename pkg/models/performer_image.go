package models

// PerformerImage describes an alternate portrait without loading its bytes.
type PerformerImage struct {
	Checksum string `db:"image_blob"`
	Position int    `db:"position"`
}
