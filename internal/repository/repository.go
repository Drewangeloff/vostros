package repository

import (
	"context"
	"time"

	"github.com/drewangeloff/vostros/internal/model"
)

type Repository interface {
	// Users
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	UpdateUserRole(ctx context.Context, userID string, role string) error
	SearchUsers(ctx context.Context, query string, limit int) ([]*model.User, error)

	// User Stats
	GetUserStats(ctx context.Context, userID string) (*model.UserStats, error)

	// Tweets
	CreateTweetWithOutbox(ctx context.Context, tweet *model.Tweet) error
	GetTweetByID(ctx context.Context, id string) (*model.Tweet, error)
	DeleteTweet(ctx context.Context, id string) error
	UpdateTweetStatus(ctx context.Context, id string, status string) error
	GetTweetsByUserID(ctx context.Context, userID string, cursor string, limit int) ([]*model.Tweet, string, error)

	// Timeline
	GetHomeTimeline(ctx context.Context, userID string, cursor string, limit int) ([]*model.Tweet, string, error)
	GetGlobalTimeline(ctx context.Context, cursor string, limit int) ([]*model.Tweet, string, error)
	InsertTimelineEntries(ctx context.Context, userIDs []string, tweetID string, createdAt time.Time) error
	DeleteTimelineEntries(ctx context.Context, tweetID string) error

	// Follows
	Follow(ctx context.Context, followerID, followingID string) error
	Unfollow(ctx context.Context, followerID, followingID string) error
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
	GetFollowers(ctx context.Context, userID string, cursor string, limit int) ([]*model.User, string, error)
	GetFollowing(ctx context.Context, userID string, cursor string, limit int) ([]*model.User, string, error)
	GetFollowerIDs(ctx context.Context, userID string) ([]string, error)

	// Search
	SearchTweets(ctx context.Context, query string, cursor string, limit int) ([]*model.Tweet, string, error)

	// Outbox
	ClaimOutboxEvents(ctx context.Context, batchSize int) ([]*model.OutboxEvent, error)
	CompleteOutboxEvent(ctx context.Context, id int64) error
	FailOutboxEvent(ctx context.Context, id int64) error

	// Auth tokens
	CreateRefreshToken(ctx context.Context, token *model.RefreshToken) error
	GetRefreshToken(ctx context.Context, tokenID string) (*model.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, tokenID string) error
	DeleteRefreshTokensByUser(ctx context.Context, userID string) error

	// API tokens
	CreateAPIToken(ctx context.Context, token *model.APIToken) error
	ListAPITokensByUser(ctx context.Context, userID string) ([]*model.APIToken, error)
	DeleteAPIToken(ctx context.Context, id string, userID string) error
	GetAPITokenByHash(ctx context.Context, hash string) (*model.APIToken, error)
	TouchAPIToken(ctx context.Context, id string) error

	// Agent actions
	CreateAgentAction(ctx context.Context, action *model.AgentAction) error
	GetAgentActions(ctx context.Context, since time.Time, limit int) ([]*model.AgentAction, error)

	// Health
	Ping(ctx context.Context) error
}
