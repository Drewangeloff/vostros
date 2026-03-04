package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/drewangeloff/old_school_bird/internal/model"
)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func (r *PostgresRepo) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// --- Users ---

func (r *PostgresRepo) CreateUser(ctx context.Context, user *model.User) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, username, email, password, display_name, bio, avatar_url, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, user.ID, user.Username, user.Email, user.Password, user.DisplayName, user.Bio, user.AvatarURL, user.Role, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `INSERT INTO user_stats (user_id) VALUES ($1)`, user.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepo) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx, `SELECT id, username, email, password, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE id = $1`, id))
}

func (r *PostgresRepo) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx, `SELECT id, username, email, password, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE username = $1`, username))
}

func (r *PostgresRepo) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx, `SELECT id, username, email, password, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE email = $1`, email))
}

func (r *PostgresRepo) scanUser(row pgx.Row) (*model.User, error) {
	u := &model.User{}
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *PostgresRepo) UpdateUser(ctx context.Context, user *model.User) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET display_name = $1, bio = $2, updated_at = NOW() WHERE id = $3
	`, user.DisplayName, user.Bio, user.ID)
	return err
}

func (r *PostgresRepo) SearchUsers(ctx context.Context, query string, limit int) ([]*model.User, error) {
	// Escape ILIKE special characters to prevent wildcard enumeration
	escaped := escapeLike(query)
	rows, err := r.pool.Query(ctx, `
		SELECT id, username, display_name, avatar_url, bio
		FROM users WHERE username ILIKE $1 OR display_name ILIKE $1
		ORDER BY username LIMIT $2
	`, "%"+escaped+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *PostgresRepo) UpdateUserRole(ctx context.Context, userID string, role string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`, role, userID)
	return err
}

// --- User Stats ---

func (r *PostgresRepo) GetUserStats(ctx context.Context, userID string) (*model.UserStats, error) {
	s := &model.UserStats{}
	err := r.pool.QueryRow(ctx, `SELECT user_id, follower_count, following_count, tweet_count FROM user_stats WHERE user_id = $1`, userID).
		Scan(&s.UserID, &s.FollowerCount, &s.FollowingCount, &s.TweetCount)
	if err == pgx.ErrNoRows {
		return &model.UserStats{UserID: userID}, nil
	}
	return s, err
}

// --- Tweets ---

func (r *PostgresRepo) CreateTweetWithOutbox(ctx context.Context, tweet *model.Tweet) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO tweets (id, user_id, content, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, tweet.ID, tweet.UserID, tweet.Content, tweet.Status, tweet.CreatedAt)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]string{
		"tweet_id":   tweet.ID,
		"user_id":    tweet.UserID,
		"created_at": tweet.CreatedAt.Format(time.RFC3339Nano),
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (event_type, payload) VALUES ('tweet_created', $1)
	`, payload)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE user_stats SET tweet_count = tweet_count + 1 WHERE user_id = $1
	`, tweet.UserID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepo) GetTweetByID(ctx context.Context, id string) (*model.Tweet, error) {
	t := &model.Tweet{User: &model.User{}}
	err := r.pool.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.content, t.status, t.created_at,
		       u.id, u.username, u.display_name, u.avatar_url
		FROM tweets t JOIN users u ON t.user_id = u.id
		WHERE t.id = $1
	`, id).Scan(&t.ID, &t.UserID, &t.Content, &t.Status, &t.CreatedAt,
		&t.User.ID, &t.User.Username, &t.User.DisplayName, &t.User.AvatarURL)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (r *PostgresRepo) DeleteTweet(ctx context.Context, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get user_id before soft-deleting
	var userID string
	err = tx.QueryRow(ctx, `UPDATE tweets SET status = 'deleted' WHERE id = $1 AND status = 'visible' RETURNING user_id`, id).Scan(&userID)
	if err != nil {
		return err
	}

	// Decrement tweet count
	_, err = tx.Exec(ctx, `UPDATE user_stats SET tweet_count = GREATEST(tweet_count - 1, 0) WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}

	// Create outbox event for timeline cleanup
	payload, _ := json.Marshal(map[string]string{"tweet_id": id})
	_, err = tx.Exec(ctx, `INSERT INTO outbox (event_type, payload) VALUES ('tweet_deleted', $1)`, payload)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepo) UpdateTweetStatus(ctx context.Context, id string, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tweets SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (r *PostgresRepo) GetTweetsByUserID(ctx context.Context, userID string, cursor string, limit int) ([]*model.Tweet, string, error) {
	query := `
		SELECT t.id, t.user_id, t.content, t.status, t.created_at,
		       u.id, u.username, u.display_name, u.avatar_url
		FROM tweets t JOIN users u ON t.user_id = u.id
		WHERE t.user_id = $1 AND t.status = 'visible'
	`
	args := []any{userID}
	if cursor != "" {
		query += ` AND t.id < $3`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	query += ` ORDER BY t.id DESC LIMIT $2`
	return r.scanTweets(ctx, query, args...)
}

// --- Timeline ---

func (r *PostgresRepo) GetGlobalTimeline(ctx context.Context, cursor string, limit int) ([]*model.Tweet, string, error) {
	query := `
		SELECT t.id, t.user_id, t.content, t.status, t.created_at,
		       u.id, u.username, u.display_name, u.avatar_url
		FROM tweets t JOIN users u ON t.user_id = u.id
		WHERE t.status = 'visible'
	`
	args := []any{}
	if cursor != "" {
		query += ` AND t.id < $2`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	query += ` ORDER BY t.id DESC LIMIT $1`
	return r.scanTweets(ctx, query, args...)
}

func (r *PostgresRepo) GetHomeTimeline(ctx context.Context, userID string, cursor string, limit int) ([]*model.Tweet, string, error) {
	query := `
		SELECT t.id, t.user_id, t.content, t.status, t.created_at,
		       u.id, u.username, u.display_name, u.avatar_url
		FROM timeline_entries te
		JOIN tweets t ON te.tweet_id = t.id
		JOIN users u ON t.user_id = u.id
		WHERE te.user_id = $1 AND t.status = 'visible'
	`
	args := []any{userID}
	if cursor != "" {
		query += ` AND t.id < $3`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	query += ` ORDER BY t.id DESC LIMIT $2`
	return r.scanTweets(ctx, query, args...)
}

func (r *PostgresRepo) InsertTimelineEntries(ctx context.Context, userIDs []string, tweetID string, createdAt time.Time) error {
	if len(userIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, uid := range userIDs {
		batch.Queue(`INSERT INTO timeline_entries (user_id, tweet_id, created_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, uid, tweetID, createdAt)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range userIDs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepo) DeleteTimelineEntries(ctx context.Context, tweetID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM timeline_entries WHERE tweet_id = $1`, tweetID)
	return err
}

// --- Follows ---

func (r *PostgresRepo) Follow(ctx context.Context, followerID, followingID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `INSERT INTO follows (follower_id, following_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, followerID, followingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Already following — don't inflate counters
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `UPDATE user_stats SET following_count = following_count + 1 WHERE user_id = $1`, followerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE user_stats SET follower_count = follower_count + 1 WHERE user_id = $1`, followingID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepo) Unfollow(ctx context.Context, followerID, followingID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`, followerID, followingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `UPDATE user_stats SET following_count = GREATEST(following_count - 1, 0) WHERE user_id = $1`, followerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE user_stats SET follower_count = GREATEST(follower_count - 1, 0) WHERE user_id = $1`, followingID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepo) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2)`, followerID, followingID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepo) GetFollowers(ctx context.Context, userID string, cursor string, limit int) ([]*model.User, string, error) {
	query := `
		SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio
		FROM follows f JOIN users u ON f.follower_id = u.id
		WHERE f.following_id = $1
	`
	args := []any{userID}
	if cursor != "" {
		query += ` AND u.id < $3`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	query += ` ORDER BY u.id DESC LIMIT $2`
	return r.scanUsers(ctx, query, args...)
}

func (r *PostgresRepo) GetFollowing(ctx context.Context, userID string, cursor string, limit int) ([]*model.User, string, error) {
	query := `
		SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio
		FROM follows f JOIN users u ON f.following_id = u.id
		WHERE f.follower_id = $1
	`
	args := []any{userID}
	if cursor != "" {
		query += ` AND u.id < $3`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	query += ` ORDER BY u.id DESC LIMIT $2`
	return r.scanUsers(ctx, query, args...)
}

func (r *PostgresRepo) GetFollowerIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT follower_id FROM follows WHERE following_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Search ---

func (r *PostgresRepo) SearchTweets(ctx context.Context, query string, cursor string, limit int) ([]*model.Tweet, string, error) {
	sql := `
		SELECT t.id, t.user_id, t.content, t.status, t.created_at,
		       u.id, u.username, u.display_name, u.avatar_url
		FROM tweets t JOIN users u ON t.user_id = u.id
		WHERE t.status = 'visible' AND t.search_vec @@ plainto_tsquery('english', $1)
	`
	args := []any{query}
	if cursor != "" {
		sql += ` AND t.id < $3`
		args = append(args, limit, cursor)
	} else {
		args = append(args, limit)
	}
	sql += ` ORDER BY t.id DESC LIMIT $2`
	return r.scanTweets(ctx, sql, args...)
}

// --- Outbox ---

func (r *PostgresRepo) ClaimOutboxEvents(ctx context.Context, batchSize int) ([]*model.OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE outbox SET status = 'processing', attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM outbox WHERE status = 'pending' ORDER BY created_at LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, payload, status, attempts, created_at
	`, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*model.OutboxEvent
	for rows.Next() {
		e := &model.OutboxEvent{}
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.Status, &e.Attempts, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *PostgresRepo) CompleteOutboxEvent(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE outbox SET status = 'done', processed_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *PostgresRepo) FailOutboxEvent(ctx context.Context, id int64) error {
	// Move to dead-letter after 5 attempts to prevent infinite retries
	_, err := r.pool.Exec(ctx, `
		UPDATE outbox SET status = CASE WHEN attempts >= 5 THEN 'dead' ELSE 'pending' END
		WHERE id = $1
	`, id)
	return err
}

// --- Auth tokens ---

func (r *PostgresRepo) CreateRefreshToken(ctx context.Context, token *model.RefreshToken) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO refresh_tokens (id, user_id, expires_at) VALUES ($1, $2, $3)`, token.ID, token.UserID, token.ExpiresAt)
	return err
}

func (r *PostgresRepo) GetRefreshToken(ctx context.Context, tokenID string) (*model.RefreshToken, error) {
	t := &model.RefreshToken{}
	err := r.pool.QueryRow(ctx, `SELECT id, user_id, expires_at, created_at FROM refresh_tokens WHERE id = $1`, tokenID).
		Scan(&t.ID, &t.UserID, &t.ExpiresAt, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (r *PostgresRepo) DeleteRefreshToken(ctx context.Context, tokenID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, tokenID)
	return err
}

func (r *PostgresRepo) DeleteRefreshTokensByUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	return err
}

// --- Agent actions ---

func (r *PostgresRepo) CreateAgentAction(ctx context.Context, action *model.AgentAction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO agent_actions (id, agent_id, action_type, target_type, target_id, reasoning, reversible)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, action.ID, action.AgentID, action.ActionType, action.TargetType, action.TargetID, action.Reasoning, action.Reversible)
	return err
}

func (r *PostgresRepo) GetAgentActions(ctx context.Context, since time.Time, limit int) ([]*model.AgentAction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, action_type, target_type, target_id, reasoning, reversible, reversed, created_at
		FROM agent_actions WHERE created_at >= $1 ORDER BY created_at DESC LIMIT $2
	`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []*model.AgentAction
	for rows.Next() {
		a := &model.AgentAction{}
		if err := rows.Scan(&a.ID, &a.AgentID, &a.ActionType, &a.TargetType, &a.TargetID, &a.Reasoning, &a.Reversible, &a.Reversed, &a.CreatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// --- Helpers ---

func (r *PostgresRepo) scanTweets(ctx context.Context, query string, args ...any) ([]*model.Tweet, string, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var tweets []*model.Tweet
	var nextCursor string
	for rows.Next() {
		t := &model.Tweet{User: &model.User{}}
		if err := rows.Scan(&t.ID, &t.UserID, &t.Content, &t.Status, &t.CreatedAt,
			&t.User.ID, &t.User.Username, &t.User.DisplayName, &t.User.AvatarURL); err != nil {
			return nil, "", err
		}
		tweets = append(tweets, t)
		nextCursor = t.ID
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("scanning tweets: %w", err)
	}
	return tweets, nextCursor, nil
}

func (r *PostgresRepo) scanUsers(ctx context.Context, query string, args ...any) ([]*model.User, string, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var users []*model.User
	var nextCursor string
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio); err != nil {
			return nil, "", err
		}
		users = append(users, u)
		nextCursor = u.ID
	}
	return users, nextCursor, rows.Err()
}

// escapeLike escapes ILIKE special characters to prevent wildcard injection
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
