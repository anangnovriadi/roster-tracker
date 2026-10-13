CREATE INDEX IF NOT EXISTS idx_note_creator_id ON note(creator_id);
CREATE INDEX IF NOT EXISTS idx_attachment_creator_id ON attachment(creator_id);
