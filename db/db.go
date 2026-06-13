package db

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/z46-dev/golog"
	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/overlord-ipa/conf"
)

var (
	log            *golog.Logger
	driver         *gosqlite.Driver
	Jobs           *gosqlite.RegisteredStruct[Job]
	JobActions     *gosqlite.RegisteredStruct[JobAction]
	JobRuns        *gosqlite.RegisteredStruct[JobRun]
	JobActionRuns  *gosqlite.RegisteredStruct[JobActionRun]
	JobHostResults *gosqlite.RegisteredStruct[JobHostResult]
	Hosts          *gosqlite.RegisteredStruct[Host]
	migrationOpts  gosqlite.MigrationOptions
)

// registerAndMigrate registers a model and applies schema migrations.
func registerAndMigrate[T any]() (registered *gosqlite.RegisteredStruct[T], err error) {
	var (
		report  *gosqlite.MigrationReport
		msg     strings.Builder
		written int
	)

	if registered, err = gosqlite.Register(driver, *new(T)); err != nil {
		return
	}

	if report, err = registered.Migrate(migrationOpts); err != nil {
		return
	}

	if report == nil || (len(report.AddedColumns) == 0 && len(report.ChangedColumns) == 0 && len(report.DroppedColumns) == 0 && len(report.RenamedColumns) == 0) {
		return
	}

	if written, err = fmt.Fprintf(&msg, "Migrating table %s...\n", report.Table); err != nil || written == 0 {
		return
	}

	if len(report.AddedColumns) > 0 {
		if written, err = fmt.Fprintf(&msg, "    + Added Columns: %v\n", report.AddedColumns); err != nil || written == 0 {
			return
		}
	}

	if len(report.ChangedColumns) > 0 {
		if written, err = fmt.Fprintf(&msg, "    ~ Changed Columns: %v\n", report.ChangedColumns); err != nil || written == 0 {
			return
		}
	}

	if len(report.DroppedColumns) > 0 {
		if written, err = fmt.Fprintf(&msg, "    - Dropped Columns: %v\n", report.DroppedColumns); err != nil || written == 0 {
			return
		}
	}

	if len(report.RenamedColumns) > 0 {
		if written, err = fmt.Fprintf(&msg, "    ~ Renamed Columns: %v\n", report.RenamedColumns); err != nil || written == 0 {
			return
		}
	}

	if written, err = msg.WriteString("    - Success!\n"); err != nil || written == 0 {
		return
	}

	log.Infof("%s\n", msg.String())
	return
}

// Init opens the database and registers all persisted models.
func Init(parentLogger *golog.Logger) (err error) {
	log = parentLogger.SpawnChild().Prefix("[DB]", golog.BoldRed)

	// If CLI args include "--allow-destructive-migrations"...
	if slices.Contains(os.Args, "--allow-destructive-migrations") {
		migrationOpts.AllowDestructive = true
	}

	if driver, err = gosqlite.Begin(conf.Conf.Database.File); err != nil {
		return
	}

	if Jobs, err = registerAndMigrate[Job](); err != nil {
		return
	}

	if JobActions, err = registerAndMigrate[JobAction](); err != nil {
		return
	}

	if JobRuns, err = registerAndMigrate[JobRun](); err != nil {
		return
	}

	if JobActionRuns, err = registerAndMigrate[JobActionRun](); err != nil {
		return
	}

	if JobHostResults, err = registerAndMigrate[JobHostResult](); err != nil {
		return
	}

	if Hosts, err = registerAndMigrate[Host](); err != nil {
		return
	}

	return
}
