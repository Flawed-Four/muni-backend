// Package vectorstore stores embeddings in Chroma through its REST API.
package vectorstore

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

const maxErrorBody = 64 << 10

// ChromaStore implements domain.VectorStore and is safe for concurrent use.
type ChromaStore struct {
	client  *http.Client
	baseURL string
	dbPath  string
	apiKey  string

	mu  sync.Mutex
	ids map[string]string // collection name → ID
}

// NewChromaStore fails if Chroma is unreachable or the tenant or database does not exist.
func NewChromaStore(cfg *config.Config) (*ChromaStore, func(), error) {
	c := cfg.Chroma

	// A zero timeout means "never time out" to http.Client.
	if c.Timeout <= 0 {
		return nil, nil, fmt.Errorf("chroma timeout must be positive, got %s", c.Timeout)
	}
	if c.Tenant == "" || c.Database == "" {
		return nil, nil, errors.New("chroma tenant and database must not be empty")
	}

	scheme := "http"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if c.TLS {
		scheme = "https"
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	s := &ChromaStore{
		client: &http.Client{
			Transport: transport,
			Timeout:   c.Timeout,
			// Don't follow redirects: they would send the API key to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		baseURL: scheme + "://" + net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) + "/api/v2",
		dbPath:  "tenants/" + url.PathEscape(c.Tenant) + "/databases/" + url.PathEscape(c.Database),
		apiKey:  c.APIKey,
		ids:     make(map[string]string),
	}
	cleanup := func() { s.client.CloseIdleConnections() }

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	if err := s.do(ctx, http.MethodGet, "heartbeat", nil, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("reach chroma at %s: %w", s.baseURL, err)
	}
	if err := s.do(ctx, http.MethodGet, s.dbPath, nil, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("open chroma database %q in tenant %q: %w", c.Database, c.Tenant, err)
	}
	return s, cleanup, nil
}

type upsertRequest struct {
	IDs        []string         `json:"ids"`
	Embeddings [][]float32      `json:"embeddings"`
	Documents  []string         `json:"documents"`
	Metadatas  []map[string]any `json:"metadatas"`
}

func (s *ChromaStore) Upsert(ctx context.Context, collection string, records []domain.VectorRecord) error {
	if len(records) == 0 {
		return nil
	}

	req := upsertRequest{
		IDs:        make([]string, len(records)),
		Embeddings: make([][]float32, len(records)),
		Documents:  make([]string, len(records)),
		Metadatas:  make([]map[string]any, len(records)),
	}
	for i, r := range records {
		if r.ID == "" {
			return fmt.Errorf("chroma upsert: record %d has no ID", i)
		}
		if err := checkEmbedding(r.Embedding); err != nil {
			return fmt.Errorf("chroma upsert: record %q: %w", r.ID, err)
		}
		metadata, err := encodeMetadata(r.Metadata)
		if err != nil {
			return fmt.Errorf("chroma upsert: record %q: %w", r.ID, err)
		}
		req.IDs[i] = r.ID
		req.Embeddings[i] = r.Embedding
		// An empty document is sent too, so it clears the old one.
		req.Documents[i] = r.Document
		req.Metadatas[i] = metadata
	}

	err := s.onCollection(ctx, collection, true, func(path string) error {
		return s.do(ctx, http.MethodPost, path+"/upsert", req, nil)
	})
	if err != nil {
		return fmt.Errorf("chroma upsert: %w", err)
	}
	return nil
}

type queryRequest struct {
	QueryEmbeddings [][]float32 `json:"query_embeddings"`
	NResults        int         `json:"n_results"`
	Where           any         `json:"where,omitempty"`
	Include         []string    `json:"include"`
}

// Chroma returns one list per query embedding. We send only one, so only [0] is used.
type queryResponse struct {
	IDs       [][]string         `json:"ids"`
	Documents [][]*string        `json:"documents"`
	Metadatas [][]map[string]any `json:"metadatas"`
	Distances [][]*float32       `json:"distances"`
}

func (s *ChromaStore) Query(ctx context.Context, collection string, q domain.VectorQuery) ([]domain.VectorMatch, error) {
	if q.TopK <= 0 {
		return nil, fmt.Errorf("chroma query: top k must be positive, got %d", q.TopK)
	}
	if err := checkEmbedding(q.Embedding); err != nil {
		return nil, fmt.Errorf("chroma query: %w", err)
	}
	where, err := whereClause(q.Where)
	if err != nil {
		return nil, fmt.Errorf("chroma query: %w", err)
	}

	req := queryRequest{
		QueryEmbeddings: [][]float32{q.Embedding},
		NResults:        q.TopK,
		Where:           where,
		Include:         []string{"documents", "metadatas", "distances"},
	}
	var resp queryResponse
	err = s.onCollection(ctx, collection, false, func(path string) error {
		return s.do(ctx, http.MethodPost, path+"/query", req, &resp)
	})
	if err != nil {
		return nil, fmt.Errorf("chroma query: %w", err)
	}
	return resp.matches()
}

func (r *queryResponse) matches() ([]domain.VectorMatch, error) {
	if len(r.IDs) == 0 {
		return []domain.VectorMatch{}, nil
	}
	ids := r.IDs[0]
	if len(r.Documents) == 0 || len(r.Metadatas) == 0 || len(r.Distances) == 0 ||
		len(r.Documents[0]) != len(ids) || len(r.Metadatas[0]) != len(ids) || len(r.Distances[0]) != len(ids) {
		return nil, errors.New("chroma query: response fields do not line up with its ids")
	}

	matches := make([]domain.VectorMatch, len(ids))
	for i, id := range ids {
		metadata, err := decodeMetadata(r.Metadatas[0][i])
		if err != nil {
			return nil, fmt.Errorf("chroma query: record %q: %w", id, err)
		}
		m := domain.VectorMatch{ID: id, Metadata: metadata}
		if doc := r.Documents[0][i]; doc != nil {
			m.Document = *doc
		}
		if dist := r.Distances[0][i]; dist != nil {
			m.Distance = *dist
		}
		matches[i] = m
	}
	return matches, nil
}

type deleteRequest struct {
	IDs   []string `json:"ids,omitempty"`
	Where any      `json:"where,omitempty"`
}

func (s *ChromaStore) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if slices.Contains(ids, "") {
		return errors.New("chroma delete: empty ID")
	}
	if err := s.delete(ctx, collection, deleteRequest{IDs: ids}); err != nil {
		return fmt.Errorf("chroma delete: %w", err)
	}
	return nil
}

func (s *ChromaStore) DeleteWhere(ctx context.Context, collection string, where map[string]any) error {
	if len(where) == 0 {
		return errors.New("chroma delete where: filter must not be empty")
	}
	clause, err := whereClause(where)
	if err != nil {
		return fmt.Errorf("chroma delete where: %w", err)
	}
	if err := s.delete(ctx, collection, deleteRequest{Where: clause}); err != nil {
		return fmt.Errorf("chroma delete where: %w", err)
	}
	return nil
}

func (s *ChromaStore) delete(ctx context.Context, collection string, req deleteRequest) error {
	return s.onCollection(ctx, collection, false, func(path string) error {
		return s.do(ctx, http.MethodPost, path+"/delete", req, nil)
	})
}

// onCollection runs op on the named collection. If the collection does not
// exist and create is false, there is nothing to do and op is skipped.
// A 404 from op means the cached ID is stale (the collection was deleted), so
// the ID is looked up again and op is retried once.
func (s *ChromaStore) onCollection(ctx context.Context, name string, create bool, op func(path string) error) error {
	for retried := false; ; retried = true {
		id, found, err := s.collectionID(ctx, name, create)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}

		err = op(s.dbPath + "/collections/" + url.PathEscape(id))
		if !retried && isStatus(err, http.StatusNotFound) {
			s.forget(name, id)
			continue
		}
		return err
	}
}

type createCollectionRequest struct {
	Name          string                  `json:"name"`
	GetOrCreate   bool                    `json:"get_or_create"`
	Configuration collectionConfiguration `json:"configuration"`
}

type collectionConfiguration struct {
	HNSW hnswConfiguration `json:"hnsw"`
}

type hnswConfiguration struct {
	Space string `json:"space"`
}

type collectionResponse struct {
	ID string `json:"id"`
}

// collectionID returns the collection's ID, which record endpoints need instead of its name.
func (s *ChromaStore) collectionID(ctx context.Context, name string, create bool) (id string, found bool, err error) {
	s.mu.Lock()
	id, ok := s.ids[name]
	s.mu.Unlock()
	if ok {
		return id, true, nil
	}

	var col collectionResponse
	if create {
		err = s.do(ctx, http.MethodPost, s.dbPath+"/collections", createCollectionRequest{
			Name:          name,
			GetOrCreate:   true,
			Configuration: collectionConfiguration{HNSW: hnswConfiguration{Space: "cosine"}},
		}, &col)
		// Another caller created the same collection at the same moment.
		if isStatus(err, http.StatusConflict) {
			err = s.getCollection(ctx, name, &col)
		}
	} else {
		err = s.getCollection(ctx, name, &col)
		if isStatus(err, http.StatusNotFound) {
			return "", false, nil
		}
	}
	if err != nil {
		return "", false, fmt.Errorf("collection %q: %w", name, err)
	}
	if col.ID == "" {
		return "", false, fmt.Errorf("collection %q: response has no id", name)
	}

	s.mu.Lock()
	s.ids[name] = col.ID
	s.mu.Unlock()
	return col.ID, true, nil
}

func (s *ChromaStore) getCollection(ctx context.Context, name string, out *collectionResponse) error {
	return s.do(ctx, http.MethodGet, s.dbPath+"/collections/"+url.PathEscape(name), nil, out)
}

// forget drops a cached ID unless another caller has already replaced it.
func (s *ChromaStore) forget(name, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids[name] == id {
		delete(s.ids, name)
	}
}

type chromaError struct {
	Status  int
	Kind    string // for example NotFoundError or InvalidArgumentError
	Message string
}

func (e *chromaError) Error() string {
	if e.Kind == "" {
		return fmt.Sprintf("status %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("status %d: %s: %s", e.Status, e.Kind, e.Message)
}

func isStatus(err error, status int) bool {
	var ce *chromaError
	return errors.As(err, &ce) && ce.Status == status
}

func (s *ChromaStore) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+"/"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.apiKey != "" {
		req.Header.Set("x-chroma-token", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readError(resp)
	}
	if out == nil {
		// Drain the body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber() // keeps integers and floats apart, see decodeMetadata
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func readError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	e := &chromaError{Status: resp.StatusCode}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &body) == nil && (body.Error != "" || body.Message != "") {
		e.Kind, e.Message = body.Error, body.Message
	} else {
		e.Message = strings.TrimSpace(string(raw))
	}
	return e
}

func checkEmbedding(embedding []float32) error {
	if len(embedding) == 0 {
		return errors.New("embedding is empty")
	}
	for _, v := range embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return errors.New("embedding contains NaN or Inf")
		}
	}
	return nil
}

func encodeMetadata(metadata map[string]any) (map[string]any, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(metadata))
	for k, v := range metadata {
		if err := checkKey(k); err != nil {
			return nil, fmt.Errorf("metadata %w", err)
		}
		value, err := scalar(v)
		if err != nil {
			return nil, fmt.Errorf("metadata %q: %w", k, err)
		}
		out[k] = value
	}
	return out, nil
}

// whereClause sorts the keys so the same filter always builds the same request.
func whereClause(where map[string]any) (any, error) {
	if len(where) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(where))
	for k := range where {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	clauses := make([]map[string]any, len(keys))
	for i, k := range keys {
		if err := checkKey(k); err != nil {
			return nil, fmt.Errorf("where %w", err)
		}
		value, err := scalar(where[k])
		if err != nil {
			return nil, fmt.Errorf("where %q: %w", k, err)
		}
		clauses[i] = map[string]any{k: map[string]any{"$eq": value}}
	}
	if len(clauses) == 1 {
		return clauses[0], nil
	}
	return map[string]any{"$and": clauses}, nil
}

// checkKey rejects keys starting with $ (Chroma operators) or # (reserved by Chroma).
func checkKey(k string) error {
	if k == "" {
		return errors.New("key is empty")
	}
	if strings.HasPrefix(k, "$") || strings.HasPrefix(k, "#") {
		return fmt.Errorf("key %q must not start with $ or #", k)
	}
	return nil
}

func scalar(v any) (any, error) {
	switch v := v.(type) {
	case string, bool, int64:
		return v, nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case float32:
		return checkFloat(float64(v))
	case float64:
		return checkFloat(v)
	default:
		return nil, fmt.Errorf("unsupported type %T, want string, bool, int, int32, int64, float32 or float64", v)
	}
}

func checkFloat(f float64) (any, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, errors.New("value is NaN or Inf")
	}
	return jsonFloat(f), nil
}

// jsonFloat keeps the decimal point in whole numbers. encoding/json writes 3.0
// as 3, which Chroma would store and return as an integer.
type jsonFloat float64

func (f jsonFloat) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(float64(f))
	if err != nil {
		return nil, err
	}
	if !bytes.ContainsAny(b, ".eE") {
		b = append(b, ".0"...)
	}
	return b, nil
}

// decodeMetadata turns numbers into int64 or float64. Chroma always writes
// floats with a decimal point (3.0), so a number without one is an integer.
func decodeMetadata(metadata map[string]any) (map[string]any, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(metadata))
	for k, v := range metadata {
		n, ok := v.(json.Number)
		if !ok {
			out[k] = v
			continue
		}
		if !strings.ContainsAny(n.String(), ".eE") {
			i, err := n.Int64()
			if err != nil {
				return nil, fmt.Errorf("metadata %q: %w", k, err)
			}
			out[k] = i
			continue
		}
		f, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("metadata %q: %w", k, err)
		}
		out[k] = f
	}
	return out, nil
}
