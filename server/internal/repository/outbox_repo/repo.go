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
