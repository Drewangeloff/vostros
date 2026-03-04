package fanout

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/drewangeloff/vostros/internal/repository"
)

type OutboxProcessor struct {
	repo     repository.Repository
	interval time.Duration
}

func New(repo repository.Repository, interval time.Duration) *OutboxProcessor {
	return &OutboxProcessor{repo: repo, interval: interval}
}

func (p *OutboxProcessor) Start(ctx context.Context) {
	log.Printf("outbox processor started (interval: %s)", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("outbox processor stopped")
			return
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

func (p *OutboxProcessor) processBatch(ctx context.Context) {
	events, err := p.repo.ClaimOutboxEvents(ctx, 50)
	if err != nil {
		log.Printf("outbox: error claiming events: %v", err)
		return
	}
	for _, event := range events {
		switch event.EventType {
		case "tweet_created":
			p.handlePostCreated(ctx, event.ID, event.Payload)
		case "tweet_deleted":
			p.handlePostDeleted(ctx, event.ID, event.Payload)
		default:
			log.Printf("outbox: unknown event type: %s", event.EventType)
			if err := p.repo.CompleteOutboxEvent(ctx, event.ID); err != nil {
				log.Printf("outbox: error completing unknown event %d: %v", event.ID, err)
			}
		}
	}
}

func (p *OutboxProcessor) handlePostCreated(ctx context.Context, eventID int64, payload string) {
	var data struct {
		PostID    string `json:"tweet_id"`
		UserID    string `json:"user_id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		log.Printf("outbox: invalid payload for post created: %v", err)
		p.repo.FailOutboxEvent(ctx, eventID)
		return
	}

	createdAt, err := time.Parse(time.RFC3339Nano, data.CreatedAt)
	if err != nil {
		log.Printf("outbox: invalid created_at in post created payload: %v", err)
		p.repo.FailOutboxEvent(ctx, eventID)
		return
	}

	followerIDs, err := p.repo.GetFollowerIDs(ctx, data.UserID)
	if err != nil {
		log.Printf("outbox: error getting followers for %s: %v", data.UserID, err)
		p.repo.FailOutboxEvent(ctx, eventID)
		return
	}

	if len(followerIDs) > 0 {
		if err := p.repo.InsertTimelineEntries(ctx, followerIDs, data.PostID, createdAt); err != nil {
			log.Printf("outbox: error inserting timeline entries: %v", err)
			p.repo.FailOutboxEvent(ctx, eventID)
			return
		}
	}

	if err := p.repo.CompleteOutboxEvent(ctx, eventID); err != nil {
		log.Printf("outbox: error completing event %d: %v", eventID, err)
	}
}

func (p *OutboxProcessor) handlePostDeleted(ctx context.Context, eventID int64, payload string) {
	var data struct {
		PostID string `json:"tweet_id"`
	}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		log.Printf("outbox: invalid payload for post deleted: %v", err)
		p.repo.FailOutboxEvent(ctx, eventID)
		return
	}

	if err := p.repo.DeleteTimelineEntries(ctx, data.PostID); err != nil {
		log.Printf("outbox: error deleting timeline entries for post %s: %v", data.PostID, err)
		p.repo.FailOutboxEvent(ctx, eventID)
		return
	}
	if err := p.repo.CompleteOutboxEvent(ctx, eventID); err != nil {
		log.Printf("outbox: error completing event %d: %v", eventID, err)
	}
}
