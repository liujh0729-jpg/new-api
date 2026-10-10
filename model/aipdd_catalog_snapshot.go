package model

import "time"

// AIPDDCatalogSnapshot is a singleton last-known-good catalog. Payload is kept
// as TEXT for SQLite, MySQL and PostgreSQL compatibility.
type AIPDDCatalogSnapshot struct {
	ID            int       `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SchemaVersion int       `json:"schema_version"`
	Revision      string    `json:"revision" gorm:"type:varchar(128);not null"`
	SourceBaseURL string    `json:"source_base_url" gorm:"type:varchar(512);not null"`
	Payload       string    `json:"payload" gorm:"type:text;not null"`
	SyncedAt      time.Time `json:"synced_at"`
}
