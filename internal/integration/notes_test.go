package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Flawed-Four/muni-backend/internal/delivery/http/dto"
)

func TestNotesCRUD(t *testing.T) {
	resetDB(t)

	register(t, "Alice", "alice@example.com", "s3cretpass")
	token := login(t, "alice@example.com", "s3cretpass")

	// 1. Create a note (without tags)
	createReq := dto.CreateNoteRequest{
		Title:   "My First Note",
		Content: "This is a test note content.",
	}
	rec := do(t, http.MethodPost, "/api/v1/notes", createReq, token)
	wantStatus(t, rec, http.StatusCreated)
	createdNote := data[dto.NoteResponse](t, rec)

	if createdNote.ID == "" {
		t.Fatal("expected generated note ID")
	}
	if createdNote.Title != createReq.Title || createdNote.Content != createReq.Content {
		t.Fatalf("unexpected note data: %+v", createdNote)
	}
	if len(createdNote.Tags) != 0 {
		t.Fatalf("expected 0 tags on create, got %d", len(createdNote.Tags))
	}

	// 2. Add tags via tags endpoint
	for _, tagTitle := range []string{"work", "golang"} {
		tagRec := do(t, http.MethodPost, "/api/v1/tags", dto.CreateNoteTagRequest{
			NoteID: createdNote.ID,
			Title:  tagTitle,
		}, token)
		wantStatus(t, tagRec, http.StatusCreated)
	}

	// 3. Get note by ID (should include tags)
	rec = do(t, http.MethodGet, "/api/v1/notes/"+createdNote.ID, nil, token)
	wantStatus(t, rec, http.StatusOK)
	gotNote := data[dto.NoteResponse](t, rec)
	if gotNote.ID != createdNote.ID || len(gotNote.Tags) != 2 {
		t.Fatalf("unexpected got note: %+v (tags: %d)", gotNote, len(gotNote.Tags))
	}

	// 4. List notes with pagination (verifying page, limit, total, total_pages)
	rec = do(t, http.MethodGet, "/api/v1/notes?page=1&limit=10", nil, token)
	wantStatus(t, rec, http.StatusOK)
	listResp := data[dto.NoteListResponse](t, rec)
	if listResp.Total != 1 || len(listResp.Items) != 1 {
		t.Fatalf("expected 1 note, got %d (total %d)", len(listResp.Items), listResp.Total)
	}
	if listResp.Page != 1 || listResp.Limit != 10 || listResp.TotalPages != 1 {
		t.Fatalf("unexpected pagination: page=%d limit=%d total_pages=%d", listResp.Page, listResp.Limit, listResp.TotalPages)
	}
	if len(listResp.Items[0].Tags) != 2 {
		t.Fatalf("expected 2 tags in list item, got %d", len(listResp.Items[0].Tags))
	}

	// 5. Update note (without tags in request)
	updateReq := dto.UpdateNoteRequest{
		Title:   "Updated Note Title",
		Content: "Updated content",
	}
	rec = do(t, http.MethodPut, "/api/v1/notes/"+createdNote.ID, updateReq, token)
	wantStatus(t, rec, http.StatusOK)
	updatedNote := data[dto.NoteResponse](t, rec)
	if updatedNote.Title != "Updated Note Title" || updatedNote.Content != "Updated content" {
		t.Fatalf("unexpected updated note: %+v", updatedNote)
	}
	if len(updatedNote.Tags) != 2 {
		t.Fatalf("expected tags to be retained in response, got %d", len(updatedNote.Tags))
	}

	// 6. List tags of note
	rec = do(t, http.MethodGet, "/api/v1/notes/"+createdNote.ID+"/tags", nil, token)
	wantStatus(t, rec, http.StatusOK)
	tags := data[[]dto.NoteTagResponse](t, rec)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	// 7. Delete note
	rec = do(t, http.MethodDelete, "/api/v1/notes/"+createdNote.ID, nil, token)
	wantStatus(t, rec, http.StatusNoContent)

	// 8. Verify note is deleted
	rec = do(t, http.MethodGet, "/api/v1/notes/"+createdNote.ID, nil, token)
	wantStatus(t, rec, http.StatusNotFound)
}

func TestNotesOwnership(t *testing.T) {
	resetDB(t)

	register(t, "Alice", "alice@example.com", "s3cretpass")
	aliceToken := login(t, "alice@example.com", "s3cretpass")

	register(t, "Bob", "bob@example.com", "s3cretpass")
	bobToken := login(t, "bob@example.com", "s3cretpass")

	// Alice creates a note
	rec := do(t, http.MethodPost, "/api/v1/notes", dto.CreateNoteRequest{
		Title:   "Alice Private Note",
		Content: "Secret",
	}, aliceToken)
	wantStatus(t, rec, http.StatusCreated)
	aliceNote := data[dto.NoteResponse](t, rec)

	// Bob attempts to get Alice's note
	rec = do(t, http.MethodGet, "/api/v1/notes/"+aliceNote.ID, nil, bobToken)
	wantStatus(t, rec, http.StatusNotFound)

	// Bob attempts to update Alice's note
	rec = do(t, http.MethodPut, "/api/v1/notes/"+aliceNote.ID, dto.UpdateNoteRequest{
		Title:   "Hacked",
		Content: "Hacked content",
	}, bobToken)
	wantStatus(t, rec, http.StatusNotFound)

	// Bob attempts to delete Alice's note
	rec = do(t, http.MethodDelete, "/api/v1/notes/"+aliceNote.ID, nil, bobToken)
	wantStatus(t, rec, http.StatusNotFound)

	// Bob's list should be empty
	rec = do(t, http.MethodGet, "/api/v1/notes", nil, bobToken)
	wantStatus(t, rec, http.StatusOK)
	listResp := data[dto.NoteListResponse](t, rec)
	if listResp.Total != 0 {
		t.Fatalf("Bob should have 0 notes, got %d", listResp.Total)
	}
}

func TestNoteTagsCRUD(t *testing.T) {
	resetDB(t)

	register(t, "Alice", "alice@example.com", "s3cretpass")
	token := login(t, "alice@example.com", "s3cretpass")

	// Create note
	rec := do(t, http.MethodPost, "/api/v1/notes", dto.CreateNoteRequest{
		Title:   "Note For Tags",
		Content: "Content",
	}, token)
	wantStatus(t, rec, http.StatusCreated)
	note := data[dto.NoteResponse](t, rec)

	// 1. Create tag
	tagReq := dto.CreateNoteTagRequest{
		NoteID: note.ID,
		Title:  "important",
	}
	rec = do(t, http.MethodPost, "/api/v1/tags", tagReq, token)
	wantStatus(t, rec, http.StatusCreated)
	tag := data[dto.NoteTagResponse](t, rec)
	if tag.ID == "" || tag.Title != "important" || tag.NoteID != note.ID {
		t.Fatalf("unexpected tag response: %+v", tag)
	}

	// 2. Duplicate tag on same note returns 409 Conflict
	rec = do(t, http.MethodPost, "/api/v1/tags", tagReq, token)
	wantStatus(t, rec, http.StatusConflict)

	// 3. Get tag by ID
	rec = do(t, http.MethodGet, "/api/v1/tags/"+tag.ID, nil, token)
	wantStatus(t, rec, http.StatusOK)
	gotTag := data[dto.NoteTagResponse](t, rec)
	if gotTag.ID != tag.ID || gotTag.Title != "important" {
		t.Fatalf("unexpected tag: %+v", gotTag)
	}

	// 4. Update tag
	rec = do(t, http.MethodPut, "/api/v1/tags/"+tag.ID, dto.UpdateNoteTagRequest{
		Title: "urgent",
	}, token)
	wantStatus(t, rec, http.StatusOK)
	updatedTag := data[dto.NoteTagResponse](t, rec)
	if updatedTag.Title != "urgent" {
		t.Fatalf("unexpected updated tag: %+v", updatedTag)
	}

	// 5. List tags with filter
	rec = do(t, http.MethodGet, fmt.Sprintf("/api/v1/tags?note_id=%s", note.ID), nil, token)
	wantStatus(t, rec, http.StatusOK)
	tags := data[[]dto.NoteTagResponse](t, rec)
	if len(tags) != 1 || tags[0].Title != "urgent" {
		t.Fatalf("expected 1 tag with title urgent, got %+v", tags)
	}

	// 6. Delete tag
	rec = do(t, http.MethodDelete, "/api/v1/tags/"+tag.ID, nil, token)
	wantStatus(t, rec, http.StatusNoContent)

	// 7. Get tag after delete
	rec = do(t, http.MethodGet, "/api/v1/tags/"+tag.ID, nil, token)
	wantStatus(t, rec, http.StatusNotFound)
}
