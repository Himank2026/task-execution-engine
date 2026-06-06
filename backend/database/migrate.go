package database

import (
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// Migrate creates or updates tables to match our Go models. GORM's AutoMigrate
// reads each struct's tags and issues the CREATE TABLE / ADD COLUMN / ADD INDEX
// statements needed to make the DB match — it never drops columns, so it's safe
// to run on every startup.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Task{},
		&models.DeadLetterTask{},
		&models.APIKey{},
	)
}
