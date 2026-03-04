CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    username    TEXT NOT NULL UNIQUE,
    email       TEXT NOT NULL UNIQUE,
    password    TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    bio         TEXT NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT 'user',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tweets (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    content     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'visible',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vec  TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
);
CREATE INDEX IF NOT EXISTS idx_tweets_user_id ON tweets(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tweets_created_at ON tweets(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tweets_status ON tweets(status);
CREATE INDEX IF NOT EXISTS idx_tweets_search ON tweets USING GIN(search_vec);

CREATE TABLE IF NOT EXISTS follows (
    follower_id  TEXT NOT NULL REFERENCES users(id),
    following_id TEXT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, following_id)
);
CREATE INDEX IF NOT EXISTS idx_follows_following ON follows(following_id);

CREATE TABLE IF NOT EXISTS timeline_entries (
    user_id    TEXT NOT NULL REFERENCES users(id),
    tweet_id   TEXT NOT NULL REFERENCES tweets(id),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, tweet_id)
);
CREATE INDEX IF NOT EXISTS idx_timeline_user_time ON timeline_entries(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS outbox (
    id          BIGSERIAL PRIMARY KEY,
    event_type  TEXT NOT NULL,
    payload     JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox(status, created_at);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS agent_actions (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL,
    action_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    reasoning   TEXT NOT NULL,
    reversible  BOOLEAN NOT NULL DEFAULT TRUE,
    reversed    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_actions_agent ON agent_actions(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_actions_target ON agent_actions(target_type, target_id);

CREATE TABLE IF NOT EXISTS user_stats (
    user_id         TEXT PRIMARY KEY REFERENCES users(id),
    follower_count  INTEGER NOT NULL DEFAULT 0,
    following_count INTEGER NOT NULL DEFAULT 0,
    tweet_count     INTEGER NOT NULL DEFAULT 0
);
