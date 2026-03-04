package ctxutil

import (
	"context"

	"github.com/drewangeloff/old_school_bird/internal/model"
)

type contextKey string

const userContextKey contextKey = "user"

func SetUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func GetUser(ctx context.Context) *model.User {
	u, _ := ctx.Value(userContextKey).(*model.User)
	return u
}
