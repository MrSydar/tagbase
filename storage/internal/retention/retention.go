package retention

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"mrsydar/tagbase/storage/internal/db"
	"mrsydar/tagbase/storage/internal/storage"
)

// Sweeper periodically deletes expired objects.
type Sweeper struct {
	db       *db.DB
	store    *storage.S3Store
	interval time.Duration
	logger   *zap.Logger
}

// NewSweeper creates a new retention sweeper.
func NewSweeper(database *db.DB, store *storage.S3Store, interval time.Duration, logger *zap.Logger) *Sweeper {
	return &Sweeper{
		db:       database,
		store:    store,
		interval: interval,
		logger:   logger,
	}
}

// Start begins the background sweep loop.
func (s *Sweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sweep(ctx); err != nil {
				s.logger.Error("retention sweep failed", zap.Error(err))
			}
		}
	}
}

func (s *Sweeper) sweep(ctx context.Context) error {
	ids, err := s.db.ListExpiredObjects(ctx, 100)
	if err != nil {
		return fmt.Errorf("list expired objects: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		payloadKey, err := s.db.DeleteObject(ctx, id)
		if err != nil {
			s.logger.Warn("failed to delete expired object from db", zap.String("id", id), zap.Error(err))
			continue
		}
		if err := s.store.Delete(ctx, payloadKey); err != nil {
			s.logger.Warn("failed to delete expired object from S3", zap.String("id", id), zap.String("key", payloadKey), zap.Error(err))
			continue
		}
		s.logger.Info("deleted expired object", zap.String("id", id))
	}
	return nil
}
