package outboxrepo

import (
	"errors"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository/model"
	"gorm.io/gorm"
)

type schemaMigration struct {
	Name string `gorm:"primaryKey"`
}

func Migrate(db *gorm.DB, maxAge time.Duration) error {
	return db.Transaction(func(transaction *gorm.DB) error {
		if transaction.Dialector.Name() == "postgres" {
			if err := transaction.Exec("SELECT pg_advisory_xact_lock(70102026)").Error; err != nil {
				return err
			}
		}
		if err := transaction.AutoMigrate(&schemaMigration{}); err != nil {
			return err
		}
		var count int64
		if err := transaction.Model(&schemaMigration{}).Where("name = ?", "0.1.2-durable-delivery").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		if err := transaction.AutoMigrate(&model.Outbox{}); err != nil {
			return err
		}
		var duplicates []struct{ Count int64 }
		if err := transaction.Model(&model.Outbox{}).Select("COUNT(*) AS count").Where("message_id <> ''").Group("channel, message_id").Having("COUNT(*) > 1").Limit(1).Scan(&duplicates).Error; err != nil {
			return err
		}
		if len(duplicates) != 0 {
			return errors.New("migration requires owner reconciliation of duplicate idempotency keys")
		}
		var rows []model.Outbox
		if err := transaction.Model(&model.Outbox{}).FindInBatches(&rows, 100, func(batch *gorm.DB, _ int) error {
			for _, row := range rows {
				if row.MessageID != "" && row.Fingerprint == "" {
					return errors.New("migration requires owner reconciliation of legacy keyed submissions")
				}
				eligible := row.CreatedAt
				if row.NextRetryAt != nil {
					eligible = *row.NextRetryAt
				}
				var deadline *time.Time
				if maxAge > 0 {
					value := eligible.Add(maxAge)
					deadline = &value
				}
				attempts := 0
				if row.State != int(entity.OutboxStatePending) {
					attempts = row.RetryCount + 1
				}
				values := map[string]any{"eligible_at": eligible, "retry_deadline": deadline, "attempts": attempts}
				if row.State == int(entity.OutboxStateSending) {
					values["lease_until"] = time.Unix(0, 0).UTC()
				}
				if err := transaction.Model(&model.Outbox{}).Where("id = ?", row.ID).Updates(values).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error; err != nil {
			return err
		}
		if err := transaction.Exec("CREATE UNIQUE INDEX outbox_channel_message_key ON outboxes (channel, message_id) WHERE message_id <> ''").Error; err != nil {
			return err
		}
		if err := transaction.Exec("CREATE INDEX outbox_dispatch_due ON outboxes (channel, state, dispatch_after, next_retry_at)").Error; err != nil {
			return err
		}
		return transaction.Create(&schemaMigration{Name: "0.1.2-durable-delivery"}).Error
	})
}
