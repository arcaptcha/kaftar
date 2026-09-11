package outboxrepo

import (
	"context"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	"github.com/arcaptcha/kaftar/server/pkg/fp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type repo struct {
	db *gorm.DB
}

func New(db *gorm.DB) repository.DurableOutbox {
	return &repo{
		db: db,
	}
}

func (r *repo) Create(ctx context.Context, o *entity.Outbox) (*entity.Outbox, error) {
	outbox := model.OutboxEntityToModel(o)
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(outbox)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		var existing model.Outbox
		if err := r.db.WithContext(ctx).Where("channel = ? AND message_id = ?", outbox.Channel, outbox.MessageID).First(&existing).Error; err != nil {
			return nil, err
		}
		if outbox.MessageID == "" || existing.Fingerprint == "" || existing.Fingerprint != outbox.Fingerprint {
			return nil, repository.ErrIdempotencyConflict
		}
		return model.OutboxModelToEntity(&existing), nil
	}
	return model.OutboxModelToEntity(outbox), nil
}

func (r *repo) Get(ctx context.Context, f *entity.OutboxFilter) ([]entity.Outbox, error) {
	filter := model.OutboxFilterEntityToModel(f)
	q := filter.Query(r.db.WithContext(ctx))

	var outboxes []model.Outbox
	result := q.Find(&outboxes)
	if err := result.Error; err != nil {
		return nil, err
	}
	if result.RowsAffected == 0 {
		return nil, repository.ErrRecordNotFound
	}

	output := fp.Mapper(outboxes, func(o model.Outbox) entity.Outbox {
		return *model.OutboxModelToEntity(&o)
	})
	return output, nil
}

func (r *repo) Update(ctx context.Context, o *entity.Outbox) error {
	outbox := model.OutboxEntityToModel(o)
	return r.db.WithContext(ctx).Updates(outbox).Error
}

func (r *repo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Outbox{}, "id = ?", id).Error
}

func (r *repo) Stats(ctx context.Context) (repository.OutboxStats, error) {
	channels := []string{
		entity.ChannelBale.String(),
		entity.ChannelEmail.String(),
		entity.ChannelHTTP.String(),
		entity.ChannelMattermost.String(),
		entity.ChannelSMS.String(),
	}
	states := []int{
		int(entity.OutboxStatePending),
		int(entity.OutboxStateSending),
		int(entity.OutboxStateFailed),
		int(entity.OutboxStateSent),
		int(entity.OutboxStateDead),
	}
	var countRows []struct {
		Channel string
		State   int
		Count   int64
	}
	if err := r.db.WithContext(ctx).
		Model(&model.Outbox{}).
		Select("channel, state, COUNT(*) AS count").
		Where("channel IN ? AND state IN ?", channels, states).
		Group("channel, state").
		Scan(&countRows).Error; err != nil {
		return repository.OutboxStats{}, err
	}

	stats := repository.OutboxStats{
		Counts: make([]repository.OutboxCount, 0, len(countRows)),
	}
	for _, row := range countRows {
		stats.Counts = append(stats.Counts, repository.OutboxCount{
			Channel: entity.Channel(row.Channel),
			State:   entity.OutboxState(row.State),
			Count:   row.Count,
		})
	}

	var pendingChannels []string
	if err := r.db.WithContext(ctx).
		Model(&model.Outbox{}).
		Distinct("channel").
		Where("channel IN ?", channels).
		Where("state IN ?", []int{
			int(entity.OutboxStatePending),
			int(entity.OutboxStateFailed),
		}).
		Pluck("channel", &pendingChannels).Error; err != nil {
		return repository.OutboxStats{}, err
	}
	stats.OldestPending = make(
		[]repository.OldestPending,
		0,
		len(pendingChannels),
	)
	for _, channel := range pendingChannels {
		var row model.Outbox
		if err := r.db.WithContext(ctx).
			Select("eligible_at").
			Where("channel = ? AND state IN ?", channel, []int{
				int(entity.OutboxStatePending),
				int(entity.OutboxStateFailed),
			}).
			Order("eligible_at ASC").
			First(&row).Error; err != nil {
			return repository.OutboxStats{}, err
		}
		stats.OldestPending = append(stats.OldestPending, repository.OldestPending{
			Channel:    entity.Channel(channel),
			EligibleAt: row.EligibleAt,
		})
	}
	return stats, nil
}
