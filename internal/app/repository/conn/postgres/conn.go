package rcpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"

	"github.com/AlexHeadGU/catalog-service/internal/app/config/section"
	"github.com/AlexHeadGU/catalog-service/migration"
)

type (
	Client struct {
		_bunDB
		rawBunDB *bun.DB

		cfg section.RepositoryPostgres
	}

	_bunDB = bun.IDB
)

func (c *Client) GetRawBunDB() *bun.DB {
	return c.rawBunDB
}

func NewClient(ctx context.Context, cfg section.RepositoryPostgres) (*Client, error) {
	// Шаг 1: Сформируйте DSN
	var u url.URL
	u.Scheme = "postgres"
	u.Host = cfg.Address
	u.User = url.UserPassword(cfg.Username, cfg.Password)
	u.Path = cfg.Name

	args := make(url.Values)
	args.Set("sslmode", "disable")

	args.Set("read_timeout", fmt.Sprintf("%d", int(cfg.ReadTimeout.Seconds())))
	args.Set("write_timeout", fmt.Sprintf("%d", int(cfg.WriteTimeout.Seconds())))
	u.RawQuery = args.Encode()

	dsn := u.String()

	log.Printf(
		"read_timeout=%s write_timeout=%s",
		cfg.ReadTimeout, cfg.WriteTimeout,
	)

	// Шаг 2: Создаём sql.DB через pgdriver
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	sqlDB.SetMaxOpenConns(10)

	// Шаг 3: Создаём bun.DB
	db := bun.NewDB(sqlDB, pgdialect.New(), bun.WithDiscardUnknownColumns())

	// Шаг 4: Проверяем соединение с таймаутом 2 секунды
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	// Шаг 5: Возвращаем Client
	return &Client{
		_bunDB:   db,
		rawBunDB: db,
		cfg:      cfg,
	}, nil
}

func (c *Client) Migrate(ctx context.Context) (oldVer, newVer int64, err error) {
	migrations := migrate.NewMigrations()
	if err := migrations.Discover(migration.Postgres); err != nil {
		return 0, 0, fmt.Errorf("failed to discover migrations: %w", err)
	}

	opts := []migrate.MigratorOption{
		migrate.WithTableName(c.cfg.MigrationTable),
		migrate.WithLocksTableName(c.cfg.MigrationTable + "_lock"),
		migrate.WithMarkAppliedOnSuccess(true),
	}
	migrator := migrate.NewMigrator(c.rawBunDB, migrations, opts...)

	if err := migrator.Init(ctx); err != nil {
		return 0, 0, fmt.Errorf("init: %w", err)
	}

	applied, err := migrator.AppliedMigrations(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("get applied migrations: %w", err)
	}
	for _, mg := range applied {
		v, _ := strconv.ParseInt(mg.Name, 10, 64)
		if v > oldVer {
			oldVer = v
		}
	}

	group, err := migrator.Migrate(ctx)
	if err != nil {
		return oldVer, 0, fmt.Errorf("migrate: %w", err)
	}

	newVer = oldVer
	for _, mg := range group.Migrations {
		v, _ := strconv.ParseInt(mg.Name, 10, 64)
		if v > newVer {
			newVer = v
		}
	}

	return oldVer, newVer, nil
}
