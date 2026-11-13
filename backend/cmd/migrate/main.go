// Command migrate applies or inspects the embedded PostgreSQL migrations.
//
// Usage:
//
//	migrate up
//	migrate down
//	migrate down-to <version>
//	migrate status
//	migrate version
//	migrate check
//
// Configuration is loaded with the same rules as the API and worker
// (DATABASE_URL plus the DB_* pool/timeout settings).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	command := args[0]
	var target int64
	switch command {
	case "up", "down", "status", "version", "check":
		if len(args) != 1 {
			return usageError()
		}
	case "down-to":
		if len(args) != 2 {
			return usageError()
		}
		version, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || version < 0 {
			return fmt.Errorf("invalid target version %q", args[1])
		}
		target = version
	default:
		return usageError()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	pool, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrator, err := postgres.NewMigrator(pool.DB(), migrations.FS)
	if err != nil {
		return err
	}
	defer migrator.Close()

	switch command {
	case "up":
		applied, err := migrator.Up(ctx)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Println("already up to date")
			return nil
		}
		fmt.Printf("applied migrations: %v\n", applied)
		return nil
	case "down":
		version, err := migrator.Down(ctx)
		if err != nil {
			return err
		}
		if version == 0 {
			fmt.Println("no migrations to roll back")
			return nil
		}
		fmt.Printf("rolled back migration %d\n", version)
		return nil
	case "down-to":
		rolledBack, err := migrator.DownTo(ctx, target)
		if err != nil {
			return err
		}
		if len(rolledBack) == 0 {
			fmt.Printf("already at or below version %d\n", target)
			return nil
		}
		fmt.Printf("rolled back migrations: %v\n", rolledBack)
		return nil
	case "status":
		statuses, err := migrator.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Print(postgres.FormatStatus(statuses))
		return nil
	case "version":
		version, err := migrator.Version(ctx)
		if err != nil {
			return err
		}
		fmt.Println(version)
		return nil
	case "check":
		if err := migrator.Check(ctx); err != nil {
			return err
		}
		fmt.Println("schema is up to date")
		return nil
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: migrate <up|down|down-to <version>|status|version|check>")
}
