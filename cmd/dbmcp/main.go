package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dbmcp/dbmcp/internal/config"
	"github.com/dbmcp/dbmcp/internal/database"
	"github.com/dbmcp/dbmcp/internal/logger"
	"github.com/dbmcp/dbmcp/internal/mcp"
	"github.com/dbmcp/dbmcp/internal/permission"
	"github.com/dbmcp/dbmcp/internal/security"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	configPath := flag.String("config", "", "Path to config file (default: ~/.dbmcp/config.yaml)")
	flag.Parse()

	if *configPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		if home == "" {
			log.Fatal("cannot determine home directory, use --config flag")
		}
		*configPath = home + "/.dbmcp/config.yaml"
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Fatalf("config file not found: %s\nRun with --config to specify a custom path.", *configPath)
	}

	app, err := config.NewAppState(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dm := database.NewDriverManager()
	cfg := app.Config()
	for name, dbCfg := range cfg.DatabaseGroups.AllDatabases() {
		if err := dm.Register(name, dbCfg.Driver, dbCfg); err != nil {
			log.Printf("[warn] failed to register database %s: %v", name, err)
		}
	}

	perm := permission.NewChecker(cfg.PermissionsGroup)
	guard := security.NewSQLGuard(security.MaxSQLLength)

	auditLog, err := logger.NewAuditLogger()
	if err != nil {
		log.Fatalf("failed to create audit logger: %v", err)
	}

	err = config.StartWatcher(app, func() {
		newCfg := app.Config()
		dm.SyncFromConfig(newCfg.DatabaseGroups.AllDatabases())
		perm.UpdateFromConfig(newCfg.PermissionsGroup)
		log.Println("[config] hot-reload applied")
	})
	if err != nil {
		log.Printf("[warn] config watcher failed: %v", err)
	}

	dbmcp := mcp.New(app, dm, perm, guard, auditLog)

	log.Println("[dbmcp] starting MCP server on stdio...")
	if err := server.ServeStdio(dbmcp.Server()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
