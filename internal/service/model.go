package service

import "time"

// Spec describes a service before the server assigns a globally unique ID.
type Spec struct {
	PreferredID string
	Alias       string
	Description string
	Thumbnail   string
}

// Info is the server-side public metadata for an active service.
type Info struct {
	ID          string    `json:"id"`
	Alias       string    `json:"alias"`
	Description string    `json:"description,omitempty"`
	Thumbnail   string    `json:"thumbnail,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
	RemoteAddr  string    `json:"remote_addr"`
}
