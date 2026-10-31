CREATE TABLE notes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      VARCHAR(255) NOT NULL,
    content    TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_notes_user_id    ON notes (user_id);
CREATE INDEX idx_notes_updated_at ON notes (updated_at);
CREATE INDEX idx_notes_title      ON notes (title);

CREATE TABLE note_tags (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notes_id UUID         NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    title    VARCHAR(255) NOT NULL
);

CREATE INDEX        idx_note_tags_title          ON note_tags (title);
CREATE UNIQUE INDEX idx_note_tags_notes_id_title ON note_tags (notes_id, title);
