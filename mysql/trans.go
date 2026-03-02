package mysql

import (
	"context"
	"github.com/jmoiron/sqlx"
	"log"
	"time"
)

// WithTransaction 核心逻辑
// dbs 是一个可变参数，表示可以传 0 个、1 个或多个 *sqlx.DB
func WithTransaction(ctx context.Context, fn func(tx *sqlx.Tx) error, dbs ...*sqlx.DB) (err error) {
	var db *sqlx.DB

	// 判断逻辑：
	if len(dbs) > 0 && dbs[0] != nil {
		// 如果传入了参数，就用传入的（测试场景注入 mockDb）
		db = dbs[0]
	} else {
		// 如果没传参数，就用默认获取的（生产场景）
		db = GetDb()
	}

	// --- 以下是通用的事务逻辑 ---
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	var start time.Time
	if dbTiming {
		start = time.Now()
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
		if dbTiming {
			elapsed := time.Since(start).Milliseconds()
			if elapsed >= dbThreshold {
				log.Printf("[MySQL] transaction %dms", elapsed)
			}
		}
	}()

	return fn(tx)
}
