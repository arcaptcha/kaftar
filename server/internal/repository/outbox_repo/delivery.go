package outboxrepo

import (
	"context"
	"errors"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockRows(query *gorm.DB) *gorm.DB {
	if query.Dialector.Name() == "postgres" {
		return query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
	return query
}

func exhausted(row model.Outbox, now time.Time) bool {
	return (row.MaxRetries >= 0 && row.Attempts > row.MaxRetries) || (row.RetryDeadline != nil && !now.Before(*row.RetryDeadline))
}

func transitionValues(row model.Outbox) map[string]any {
	return map[string]any{
		"state": row.State, "attempts": row.Attempts, "retry_count": row.RetryCount,
		"lease_token": row.LeaseToken, "lease_until": row.LeaseUntil,
		"dispatch_after": row.DispatchAfter, "next_retry_at": row.NextRetryAt,
		"last_error": row.LastError, "sent_at": row.SentAt,
	}
}

func makeDead(row *model.Outbox) {
	row.State = int(entity.OutboxStateDead)
	row.LeaseToken, row.LeaseUntil, row.NextRetryAt, row.DispatchAfter = "", nil, nil, nil
	row.LastError = "delivery retry budget or age exhausted"
}

func (repo *repo) ReserveDispatch(ctx context.Context, channel entity.Channel, now time.Time, interval time.Duration, limit int) ([]entity.Outbox, error) {
	var result []entity.Outbox
	err := repo.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var rows []model.Outbox
		query := transaction.Where("channel = ?", channel.String()).Where(
			"((state IN ? AND (next_retry_at IS NULL OR next_retry_at <= ?) AND (dispatch_after IS NULL OR dispatch_after <= ?)) OR (state = ? AND (lease_until IS NULL OR lease_until <= ?)))",
			[]int{int(entity.OutboxStatePending), int(entity.OutboxStateFailed)}, now, now, int(entity.OutboxStateSending), now)
		if err := lockRows(query).Order("created_at, id").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if exhausted(row, now) {
				makeDead(&row)
			} else {
				if row.State == int(entity.OutboxStateSending) {
					row.State = int(entity.OutboxStateFailed)
					row.LastError = "previous delivery outcome unknown"
					row.LeaseToken, row.LeaseUntil = "", nil
					row.NextRetryAt = &now
				}
				dispatchAfter := now.Add(interval)
				row.DispatchAfter = &dispatchAfter
				result = append(result, *model.OutboxModelToEntity(&row))
			}
			if err := transaction.Model(&model.Outbox{}).Where("id = ?", row.ID).Updates(transitionValues(row)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (repo *repo) Claim(ctx context.Context, id string, channel entity.Channel, now time.Time, lease time.Duration) (*entity.Outbox, error) {
	var result *entity.Outbox
	err := repo.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var row model.Outbox
		err := lockRows(transaction.Where("id = ? AND channel = ?", id, channel.String())).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repository.ErrRecordNotFound
		}
		if err != nil {
			return err
		}
		if row.State != int(entity.OutboxStatePending) && row.State != int(entity.OutboxStateFailed) {
			return nil
		}
		if row.NextRetryAt != nil && now.Before(*row.NextRetryAt) {
			return nil
		}
		if exhausted(row, now) {
			makeDead(&row)
		} else {
			until := now.Add(lease)
			row.State, row.Attempts = int(entity.OutboxStateSending), row.Attempts+1
			row.RetryCount = row.Attempts - 1
			row.LeaseToken, row.LeaseUntil = uuid.NewString(), &until
			result = model.OutboxModelToEntity(&row)
		}
		return transaction.Model(&model.Outbox{}).Where("id = ?", id).Updates(transitionValues(row)).Error
	})
	return result, err
}

func (repo *repo) Finish(ctx context.Context, outbox *entity.Outbox, now time.Time) error {
	row := model.OutboxEntityToModel(outbox)
	token := row.LeaseToken
	row.LeaseToken, row.LeaseUntil, row.DispatchAfter = "", nil, nil
	if outbox.State == entity.OutboxStateSent {
		row.SentAt, row.NextRetryAt, row.LastError = &now, nil, ""
	} else if outbox.State == entity.OutboxStateDead {
		row.NextRetryAt = nil
	}
	result := repo.db.WithContext(ctx).Model(&model.Outbox{}).Where("id = ? AND state = ? AND lease_token = ? AND lease_until > ?", row.ID, int(entity.OutboxStateSending), token, now).Updates(transitionValues(*row))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return repository.ErrLeaseLost
	}
	return nil
}
