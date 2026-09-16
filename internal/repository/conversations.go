package repository

import (
	"context"
	"errors"
	"regexp"
	"strconv"

	"github.com/drewangeloff/vostros/internal/model"
)

var ErrPostUnavailable = errors.New("post is no longer available")

// Username mentions are case-sensitive, matching the existing account identity rules.
// Do not interpret email addresses, URL paths, or longer names as mentions.
var mentionPattern = regexp.MustCompile(`(^|[\s(\[{,:;!?])@([a-zA-Z0-9_]{3,20})\b`)

func MentionUsernames(content string) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, match := range mentionPattern.FindAllStringSubmatch(content, -1) {
		name := match[2]
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names
}

const threadVisible = `(t.thread_id IS NULL OR EXISTS (SELECT 1 FROM tweets root WHERE root.id=t.thread_id AND root.status='visible'))`
const postColumns = `t.id,t.user_id,t.content,t.status,t.created_at,
    u.id,u.username,u.display_name,u.avatar_url,
    t.kind,COALESCE(t.question_state,''),COALESCE(t.parent_id,''),COALESCE(t.thread_id,''),
    (SELECT count(*) FROM tweets replies WHERE replies.thread_id=t.id AND replies.status='visible')`

func postDest(p *model.Post) []any {
	return []any{&p.ID, &p.UserID, &p.Content, &p.Status, &p.CreatedAt,
		&p.User.ID, &p.User.Username, &p.User.DisplayName, &p.User.AvatarURL,
		&p.Kind, &p.QuestionState, &p.ParentID, &p.ThreadID, &p.ReplyCount}
}

func (r *PostgresRepo) GetReplies(ctx context.Context, threadID, cursor string, limit int) ([]*model.Post, string, error) {
	return r.scanPosts(ctx, `SELECT `+postColumns+` FROM tweets t JOIN users u ON u.id=t.user_id
        WHERE t.thread_id=$1 AND t.status='visible' AND `+threadVisible+`
        AND ($2='' OR t.id<$2) ORDER BY t.id DESC LIMIT $3`, threadID, cursor, limit)
}

func (r *PostgresRepo) GetQuestions(ctx context.Context, state, cursor string, limit int) ([]*model.Post, string, error) {
	return r.scanPosts(ctx, `SELECT `+postColumns+` FROM tweets t JOIN users u ON u.id=t.user_id
        WHERE t.kind='question' AND t.status='visible' AND ($1='all' OR t.question_state=$1)
        AND ($2='' OR t.id<$2) ORDER BY t.id DESC LIMIT $3`, state, cursor, limit)
}

func (r *PostgresRepo) SetQuestionState(ctx context.Context, id, ownerID, state string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE tweets SET question_state=$3
        WHERE id=$1 AND user_id=$2 AND kind='question' AND status='visible'`, id, ownerID, state)
	return tag.RowsAffected() > 0, err
}

const notificationFrom = ` FROM notifications n
    JOIN tweets t ON t.id=n.post_id JOIN users u ON u.id=t.user_id
    JOIN tweets root ON root.id=COALESCE(t.thread_id,t.id)
    JOIN users owner ON owner.id=root.user_id
    WHERE n.user_id=$1 AND t.status='visible' AND root.status='visible'`

func (r *PostgresRepo) GetNotifications(ctx context.Context, userID string, unread bool, cursor int64, limit int) ([]*model.Notification, string, error) {
	rows, err := r.pool.Query(ctx, `SELECT n.id,n.kind,n.created_at,n.read_at,`+postColumns+`,
        root.id,root.content,root.kind,COALESCE(root.question_state,''),owner.id,owner.username,owner.display_name
        `+notificationFrom+` AND (NOT $2 OR n.read_at IS NULL) AND ($3::bigint=0 OR n.id<$3)
        ORDER BY n.id DESC LIMIT $4`, userID, unread, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []*model.Notification{}
	next := ""
	for rows.Next() {
		n := &model.Notification{Post: &model.Post{User: &model.User{}}, Thread: &model.ThreadContext{User: &model.User{}}}
		dest := []any{&n.ID, &n.Kind, &n.CreatedAt, &n.ReadAt}
		dest = append(dest, postDest(n.Post)...)
		dest = append(dest, &n.Thread.ID, &n.Thread.Content, &n.Thread.Kind, &n.Thread.QuestionState, &n.Thread.User.ID, &n.Thread.User.Username, &n.Thread.User.DisplayName)
		if err := rows.Scan(dest...); err != nil {
			return nil, "", err
		}
		n.Thread.UserID = n.Thread.User.ID
		items = append(items, n)
		next = strconv.FormatInt(n.ID, 10)
	}
	if len(items) < limit {
		next = ""
	}
	return items, next, rows.Err()
}

func (r *PostgresRepo) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*)`+notificationFrom+` AND n.read_at IS NULL`, userID).Scan(&count)
	return count, err
}

func (r *PostgresRepo) ReadNotifications(ctx context.Context, userID string, ids []int64) (int64, error) {
	// Scope the write to the authenticated recipient, including for forged IDs.
	tag, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at=NOW() WHERE user_id=$1 AND id=ANY($2) AND read_at IS NULL`, userID, ids)
	return tag.RowsAffected(), err
}
