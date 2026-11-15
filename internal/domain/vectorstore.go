package domain

import "context"

type VectorRecord struct {
	ID        string
	Embedding []float32
	Document  string         // optional source text, returned with matches
	Metadata  map[string]any // values: string, bool, int, int32, int64, float32 or float64
}

type VectorQuery struct {
	Embedding []float32
	TopK      int // max number of matches, must be > 0
	// Where is optional. A record matches only if its metadata has every key in
	// Where with an equal value.
	Where map[string]any
}

type VectorMatch struct {
	ID       string
	Document string
	Metadata map[string]any // numbers come back as int64 or float64
	Distance float32        // cosine distance: 0 = same direction, 1 = unrelated, 2 = opposite
}

// VectorStore stores embeddings and finds the nearest ones. It does not create
// embeddings; callers pass the vectors.
//
// Use one collection per kind of data (e.g. "notes"), not one per user. The
// first Upsert creates the collection and fixes its vector size, so a new
// embedding model needs a new collection.
//
// If a collection holds data of several users, always filter by owner: put the
// owner in Metadata and in Where.
type VectorStore interface {
	// Upsert adds records or replaces the embedding and document of existing
	// IDs. Metadata is merged: old keys missing from the new record are kept.
	Upsert(ctx context.Context, collection string, records []VectorRecord) error
	// Query returns up to TopK matches, closest first.
	Query(ctx context.Context, collection string, q VectorQuery) ([]VectorMatch, error)
	// Delete removes records by ID. Unknown IDs and an empty list are no-ops.
	Delete(ctx context.Context, collection string, ids []string) error
	// DeleteWhere removes records whose metadata matches where. An empty where
	// is rejected so a missing filter cannot wipe the collection.
	DeleteWhere(ctx context.Context, collection string, where map[string]any) error
}
