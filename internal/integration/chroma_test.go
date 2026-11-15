package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/vectorstore"
)

func newVectorStore(t *testing.T) *vectorstore.ChromaStore {
	t.Helper()
	resetChroma(t)
	return openVectorStore(t)
}

// openVectorStore opens a store without resetting Chroma, like a second app instance.
func openVectorStore(t *testing.T) *vectorstore.ChromaStore {
	t.Helper()
	store, cleanup, err := vectorstore.NewChromaStore(testCfg)
	if err != nil {
		t.Fatalf("NewChromaStore: %v", err)
	}
	t.Cleanup(cleanup)
	return store
}

func upsert(t *testing.T, store *vectorstore.ChromaStore, collection string, records ...domain.VectorRecord) {
	t.Helper()
	if err := store.Upsert(context.Background(), collection, records); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}

func query(t *testing.T, store *vectorstore.ChromaStore, collection string, q domain.VectorQuery) []domain.VectorMatch {
	t.Helper()
	matches, err := store.Query(context.Background(), collection, q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	return matches
}

func matchIDs(matches []domain.VectorMatch) []string {
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	return ids
}

// sortedIDs is for matches at equal distance, whose order is not fixed.
func sortedIDs(matches []domain.VectorMatch) []string {
	return slices.Sorted(slices.Values(matchIDs(matches)))
}

func approx(got, want float32) bool {
	return math.Abs(float64(got-want)) < 1e-4
}

func TestChromaStore_UpsertAndQuery(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes",
		domain.VectorRecord{
			ID:        "a",
			Embedding: []float32{1, 0, 0},
			Document:  "about cats",
			Metadata: map[string]any{
				"user_id": "u1",
				"words":   42,
				"weight":  float32(0.5),
				"score":   3.0,
				"draft":   true,
			},
		},
		domain.VectorRecord{ID: "b", Embedding: []float32{0.9, 0.1, 0}, Document: "about kittens"},
		domain.VectorRecord{ID: "c", Embedding: []float32{0, 0, 1}, Document: "about taxes"},
	)

	matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0, 0}, TopK: 3})

	if got := matchIDs(matches); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("matches = %v, want [a b c], closest first", got)
	}
	a := matches[0]
	if a.Document != "about cats" {
		t.Fatalf("Document = %q, want %q", a.Document, "about cats")
	}
	// Integers come back as int64 and floats as float64, even 3.0.
	want := map[string]any{"user_id": "u1", "words": int64(42), "weight": 0.5, "score": 3.0, "draft": true}
	if !maps.Equal(a.Metadata, want) {
		t.Fatalf("Metadata = %#v, want %#v", a.Metadata, want)
	}
	if matches[1].Metadata != nil {
		t.Fatalf("Metadata of a record written without any = %#v, want nil", matches[1].Metadata)
	}
}

// Cosine distance ignores vector length; L2 distance would not.
func TestChromaStore_DistanceIsCosine(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes",
		domain.VectorRecord{ID: "same", Embedding: []float32{5, 0}},
		domain.VectorRecord{ID: "orthogonal", Embedding: []float32{0, 3}},
		domain.VectorRecord{ID: "opposite", Embedding: []float32{-2, 0}},
	)

	matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 3})

	want := map[string]float32{"same": 0, "orthogonal": 1, "opposite": 2}
	if len(matches) != len(want) {
		t.Fatalf("got %d matches, want %d", len(matches), len(want))
	}
	for _, m := range matches {
		if !approx(m.Distance, want[m.ID]) {
			t.Fatalf("distance of %s = %v, want %v", m.ID, m.Distance, want[m.ID])
		}
	}
}

func TestChromaStore_UpsertReplacesEmbeddingAndDocumentButMergesMetadata(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes", domain.VectorRecord{
		ID:        "a",
		Embedding: []float32{1, 0},
		Document:  "old",
		Metadata:  map[string]any{"user_id": "u1", "stale": "x"},
	})
	upsert(t, store, "notes", domain.VectorRecord{
		ID:        "a",
		Embedding: []float32{0, 1},
		Document:  "",
		Metadata:  map[string]any{"user_id": "u2"},
	})

	matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{0, 1}, TopK: 5})

	if len(matches) != 1 {
		t.Fatalf("got %d matches, want the one record", len(matches))
	}
	m := matches[0]
	if !approx(m.Distance, 0) {
		t.Fatalf("Distance = %v, want 0 for the new embedding", m.Distance)
	}
	if m.Document != "" {
		t.Fatalf("Document = %q, want it cleared", m.Document)
	}
	want := map[string]any{"user_id": "u2", "stale": "x"}
	if !maps.Equal(m.Metadata, want) {
		t.Fatalf("Metadata = %#v, want %#v (new keys overwrite, old ones stay)", m.Metadata, want)
	}
}

func TestChromaStore_WhereMatchesAllKeys(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes",
		domain.VectorRecord{ID: "u1-note", Embedding: []float32{1, 0}, Metadata: map[string]any{"user_id": "u1", "kind": "note", "pinned": true}},
		domain.VectorRecord{ID: "u2-note", Embedding: []float32{1, 0}, Metadata: map[string]any{"user_id": "u2", "kind": "note", "pinned": true}},
		domain.VectorRecord{ID: "u1-task", Embedding: []float32{1, 0}, Metadata: map[string]any{"user_id": "u1", "kind": "task", "pinned": false}},
		domain.VectorRecord{ID: "untagged", Embedding: []float32{1, 0}},
	)

	tests := []struct {
		where map[string]any
		want  []string
	}{
		{map[string]any{"user_id": "u1"}, []string{"u1-note", "u1-task"}},
		{map[string]any{"user_id": "u1", "kind": "note"}, []string{"u1-note"}},
		{map[string]any{"pinned": true}, []string{"u1-note", "u2-note"}},
		{map[string]any{"user_id": "u3"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.where), func(t *testing.T) {
			matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 10, Where: tt.where})
			if got := sortedIDs(matches); !slices.Equal(got, tt.want) {
				t.Fatalf("matches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChromaStore_TopKCapsMatches(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes",
		domain.VectorRecord{ID: "a", Embedding: []float32{1, 0}},
		domain.VectorRecord{ID: "b", Embedding: []float32{1, 1}},
		domain.VectorRecord{ID: "c", Embedding: []float32{0, 1}},
	)

	if got := matchIDs(query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 2})); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("TopK 2 matches = %v, want [a b]", got)
	}
	if got := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 10}); len(got) != 3 {
		t.Fatalf("TopK 10 returned %d matches, want all 3", len(got))
	}
}

func TestChromaStore_RejectsInvalidQuery(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	queries := map[string]domain.VectorQuery{
		"zero top k":         {Embedding: []float32{1}, TopK: 0},
		"negative top k":     {Embedding: []float32{1}, TopK: -1},
		"empty embedding":    {TopK: 1},
		"NaN embedding":      {Embedding: []float32{float32(math.NaN())}, TopK: 1},
		"unsupported where":  {Embedding: []float32{1}, TopK: 1, Where: map[string]any{"tags": []string{"a"}}},
		"nil where value":    {Embedding: []float32{1}, TopK: 1, Where: map[string]any{"user_id": nil}},
		"operator where key": {Embedding: []float32{1}, TopK: 1, Where: map[string]any{"$or": "x"}},
	}
	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			if _, err := store.Query(ctx, "notes", q); err == nil {
				t.Fatal("Query succeeded, want an error")
			}
		})
	}
}

// Reads and deletes must not create a collection as a side effect.
func TestChromaStore_UnknownCollectionFindsNothing(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	matches, err := store.Query(ctx, "never-written", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if matches == nil || len(matches) != 0 {
		t.Fatalf("matches = %#v, want an empty slice", matches)
	}
	if err := store.Delete(ctx, "never-written", []string{"a"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.DeleteWhere(ctx, "never-written", map[string]any{"user_id": "u1"}); err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	if got := chromaCollections(t); len(got) != 0 {
		t.Fatalf("collections = %v, want none", got)
	}
}

func TestChromaStore_DeleteByID(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	upsert(t, store, "notes",
		domain.VectorRecord{ID: "a", Embedding: []float32{1, 0}},
		domain.VectorRecord{ID: "b", Embedding: []float32{1, 0}},
		domain.VectorRecord{ID: "c", Embedding: []float32{1, 0}},
	)

	if err := store.Delete(ctx, "notes", []string{"a", "absent"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Must delete nothing, not everything.
	if err := store.Delete(ctx, "notes", nil); err != nil {
		t.Fatalf("Delete of no IDs: %v", err)
	}
	if err := store.Delete(ctx, "notes", []string{"b", ""}); err == nil {
		t.Fatal("Delete with an empty ID succeeded, want an error")
	}

	matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 10})
	if got := sortedIDs(matches); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("remaining = %v, want [b c]", got)
	}
}

func TestChromaStore_DeleteWhere(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	upsert(t, store, "notes",
		domain.VectorRecord{ID: "n1-chunk1", Embedding: []float32{1, 0}, Metadata: map[string]any{"note_id": "n1"}},
		domain.VectorRecord{ID: "n1-chunk2", Embedding: []float32{1, 0}, Metadata: map[string]any{"note_id": "n1"}},
		domain.VectorRecord{ID: "n2-chunk1", Embedding: []float32{1, 0}, Metadata: map[string]any{"note_id": "n2"}},
	)

	if err := store.DeleteWhere(ctx, "notes", map[string]any{"note_id": "n1"}); err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	for _, where := range []map[string]any{nil, {}} {
		if err := store.DeleteWhere(ctx, "notes", where); err == nil {
			t.Fatalf("DeleteWhere(%#v) succeeded, want an error", where)
		}
	}

	matches := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 10})
	if got := matchIDs(matches); !slices.Equal(got, []string{"n2-chunk1"}) {
		t.Fatalf("remaining = %v, want [n2-chunk1]", got)
	}
}

func TestChromaStore_RejectsDimensionChange(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	upsert(t, store, "notes", domain.VectorRecord{ID: "a", Embedding: []float32{1, 0}})

	if err := store.Upsert(ctx, "notes", []domain.VectorRecord{{ID: "b", Embedding: []float32{1, 0, 0}}}); err == nil {
		t.Fatal("Upsert with another dimension succeeded, want an error")
	}
	if _, err := store.Query(ctx, "notes", domain.VectorQuery{Embedding: []float32{1, 0, 0}, TopK: 1}); err == nil {
		t.Fatal("Query with another dimension succeeded, want an error")
	}
	if got := matchIDs(query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 10})); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("records = %v, want only [a]", got)
	}
}

// Validation runs before any request, so the collection is never created.
func TestChromaStore_RejectsInvalidRecords(t *testing.T) {
	store := newVectorStore(t)
	ctx := context.Background()

	records := map[string]domain.VectorRecord{
		"empty ID":           {Embedding: []float32{1}},
		"empty embedding":    {ID: "a"},
		"Inf embedding":      {ID: "a", Embedding: []float32{float32(math.Inf(1))}},
		"slice metadata":     {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"tags": []string{"x"}}},
		"struct metadata":    {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"at": time.Now()}},
		"nil metadata value": {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"user_id": nil}},
		"NaN metadata value": {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"score": math.NaN()}},
		"empty key":          {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"": "x"}},
		"reserved key":       {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"#document": "x"}},
		"operator key":       {ID: "a", Embedding: []float32{1}, Metadata: map[string]any{"$and": "x"}},
	}
	for name, r := range records {
		t.Run(name, func(t *testing.T) {
			valid := domain.VectorRecord{ID: "valid", Embedding: []float32{1}}
			if err := store.Upsert(ctx, "notes", []domain.VectorRecord{valid, r}); err == nil {
				t.Fatal("Upsert succeeded, want an error")
			}
		})
	}
	if got := chromaCollections(t); len(got) != 0 {
		t.Fatalf("collections = %v, want none", got)
	}
}

func TestChromaStore_EmptyUpsertIsNoOp(t *testing.T) {
	store := newVectorStore(t)

	if err := store.Upsert(context.Background(), "notes", nil); err != nil {
		t.Fatalf("Upsert of no records: %v", err)
	}
	if got := chromaCollections(t); len(got) != 0 {
		t.Fatalf("collections = %v, want none", got)
	}
}

// The store caches collection IDs, and a collection recreated elsewhere gets a new ID.
func TestChromaStore_RecoversFromCollectionDeletedBehindIt(t *testing.T) {
	store := newVectorStore(t)

	upsert(t, store, "notes", domain.VectorRecord{ID: "a", Embedding: []float32{1, 0}})
	resetChroma(t)

	if got := query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 5}); len(got) != 0 {
		t.Fatalf("matches after the delete = %v, want none", matchIDs(got))
	}

	upsert(t, store, "notes", domain.VectorRecord{ID: "b", Embedding: []float32{1, 0}})

	if got := matchIDs(query(t, store, "notes", domain.VectorQuery{Embedding: []float32{1, 0}, TopK: 5})); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("matches = %v, want [b]", got)
	}
}

// Several app instances create the same collection at the same moment.
func TestChromaStore_ConcurrentFirstWriteCreatesOneCollection(t *testing.T) {
	resetChroma(t)

	const writers = 20
	stores := make([]*vectorstore.ChromaStore, writers)
	for i := range stores {
		stores[i] = openVectorStore(t)
	}

	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i, store := range stores {
		wg.Go(func() {
			errs[i] = store.Upsert(context.Background(), "notes", []domain.VectorRecord{
				{ID: fmt.Sprintf("r%d", i), Embedding: []float32{1, float32(i)}},
			})
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := chromaCollections(t); !slices.Equal(got, []string{"notes"}) {
		t.Fatalf("collections = %v, want [notes]", got)
	}
	// Counted directly, not through Query: search is approximate and can miss
	// records with these tiny test vectors.
	if got := chromaCount(t, "notes"); got != writers {
		t.Fatalf("collection holds %d records, want %d", got, writers)
	}
}

func chromaCount(t *testing.T, collection string) int {
	t.Helper()
	var col struct {
		ID string `json:"id"`
	}
	getJSON(t, chromaDBURL+"/collections/"+url.PathEscape(collection), &col)
	var count int
	getJSON(t, chromaDBURL+"/collections/"+col.ID+"/count", &count)
	return count
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: decode: %v", url, err)
	}
}

func TestChromaStore_CanceledContext(t *testing.T) {
	store := newVectorStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.Upsert(ctx, "notes", []domain.VectorRecord{{ID: "a", Embedding: []float32{1}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upsert error = %v, want context.Canceled", err)
	}
	if _, err := store.Query(ctx, "notes", domain.VectorQuery{Embedding: []float32{1}, TopK: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query error = %v, want context.Canceled", err)
	}
	if got := chromaCollections(t); len(got) != 0 {
		t.Fatalf("collections = %v, want none", got)
	}
}

func TestNewChromaStore_FailsWhenServerUnreachable(t *testing.T) {
	cfg := *testCfg
	cfg.Chroma.Port = 1 // nothing listens here

	if _, _, err := vectorstore.NewChromaStore(&cfg); err == nil {
		t.Fatal("NewChromaStore succeeded, want an error")
	}
}

func TestNewChromaStore_RejectsInvalidConfig(t *testing.T) {
	tests := map[string]func(*config.ChromaConfig){
		"zero timeout":     func(c *config.ChromaConfig) { c.Timeout = 0 },
		"negative timeout": func(c *config.ChromaConfig) { c.Timeout = -time.Second },
		"empty tenant":     func(c *config.ChromaConfig) { c.Tenant = "" },
		"empty database":   func(c *config.ChromaConfig) { c.Database = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := *testCfg
			mutate(&cfg.Chroma)
			if _, _, err := vectorstore.NewChromaStore(&cfg); err == nil {
				t.Fatal("NewChromaStore succeeded, want an error")
			}
		})
	}
}

func TestNewChromaStore_FailsForUnknownDatabase(t *testing.T) {
	cfg := *testCfg
	cfg.Chroma.Database = "does_not_exist"

	if _, _, err := vectorstore.NewChromaStore(&cfg); err == nil {
		t.Fatal("NewChromaStore succeeded, want an error")
	}
}

// chroma-test is plaintext, so TLS must fail instead of falling back.
func TestNewChromaStore_TLSDoesNotFallBackToPlaintext(t *testing.T) {
	cfg := *testCfg
	cfg.Chroma.TLS = true

	if _, _, err := vectorstore.NewChromaStore(&cfg); err == nil {
		t.Fatal("NewChromaStore connected over plaintext, want an error")
	}
}

// Chroma cannot be paused on purpose, so a listener that never replies stands
// in for a hung server. This tests our timeout, not Chroma.
func TestNewChromaStore_TimeoutBoundsStalledServer(t *testing.T) {
	addr := stalledServer(t)

	const timeout = 200 * time.Millisecond
	cfg := *testCfg
	cfg.Chroma.Host = "127.0.0.1"
	cfg.Chroma.Port = addr.Port
	cfg.Chroma.Timeout = timeout

	start := time.Now()
	_, _, err := vectorstore.NewChromaStore(&cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("NewChromaStore succeeded against a server that never answers")
	}
	if elapsed > 10*timeout {
		t.Fatalf("NewChromaStore returned after %s, want it bounded by the %s timeout", elapsed, timeout)
	}
}
