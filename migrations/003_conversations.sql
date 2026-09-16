-- Additive migration: existing posts and older app revisions keep working.
ALTER TABLE tweets ADD COLUMN kind TEXT NOT NULL DEFAULT 'post'
    CHECK (kind IN ('post', 'question'));
ALTER TABLE tweets ADD COLUMN question_state TEXT
    CHECK (question_state IN ('open', 'answered', 'tested'));
ALTER TABLE tweets ADD COLUMN parent_id TEXT REFERENCES tweets(id);
ALTER TABLE tweets ADD COLUMN thread_id TEXT REFERENCES tweets(id);
ALTER TABLE tweets ADD CONSTRAINT tweets_conversation_shape CHECK (
    (parent_id IS NULL AND thread_id IS NULL) OR
    (parent_id IS NOT NULL AND thread_id IS NOT NULL AND kind = 'post')
);
ALTER TABLE tweets ADD CONSTRAINT tweets_question_shape CHECK (
    (kind = 'question' AND question_state IS NOT NULL) OR
    (kind = 'post' AND question_state IS NULL)
);
CREATE INDEX idx_tweets_thread ON tweets(thread_id, id DESC) WHERE status = 'visible';
CREATE INDEX idx_tweets_questions ON tweets(question_state, id DESC)
    WHERE kind = 'question' AND status = 'visible';

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    post_id TEXT NOT NULL REFERENCES tweets(id),
    kind TEXT NOT NULL CHECK (kind IN ('reply', 'mention')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ,
    UNIQUE (user_id, post_id)
);
CREATE INDEX idx_notifications_user ON notifications(user_id, id DESC);
CREATE INDEX idx_notifications_unread ON notifications(user_id, id DESC) WHERE read_at IS NULL;
