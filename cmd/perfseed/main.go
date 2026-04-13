package main

import (
	"flag"
	"log"

	"ampmanager/internal/database"
	"ampmanager/internal/perf/seed"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	options := seed.DefaultOptions()
	dbType := flag.String("db-type", "postgres", "database type: postgres or sqlite")
	databaseURL := flag.String("database-url", "", "database DSN for postgres")
	sqlitePath := flag.String("sqlite-path", "./data/data.db", "sqlite path")
	flag.IntVar(&options.UserCount, "user-count", options.UserCount, "number of perf users to create")
	flag.StringVar(&options.UserPrefix, "user-prefix", options.UserPrefix, "username prefix for perf users")
	flag.StringVar(&options.UserPassword, "user-password", options.UserPassword, "shared password for perf users")
	flag.StringVar(&options.AdminUsername, "admin-username", options.AdminUsername, "admin username")
	flag.StringVar(&options.AdminPassword, "admin-password", options.AdminPassword, "admin password")
	flag.Int64Var(&options.BalanceMicros, "balance-micros", options.BalanceMicros, "initial user balance in micros")
	flag.StringVar(&options.UpstreamURL, "upstream-url", options.UpstreamURL, "mock upstream base URL")
	flag.StringVar(&options.UpstreamAPIKey, "upstream-api-key", options.UpstreamAPIKey, "mock upstream API key")
	flag.BoolVar(&options.RequestDetailEnabled, "request-detail-enabled", options.RequestDetailEnabled, "whether request detail capture is enabled")
	flag.StringVar(&options.PublicChatModel, "public-chat-model", options.PublicChatModel, "public model name for chat completions")
	flag.StringVar(&options.PublicResponsesModel, "public-responses-model", options.PublicResponsesModel, "public model name for responses API")
	flag.StringVar(&options.UpstreamChatModel, "upstream-chat-model", options.UpstreamChatModel, "upstream model name for chat completions")
	flag.StringVar(&options.UpstreamResponsesModel, "upstream-responses-model", options.UpstreamResponsesModel, "upstream model name for responses API")
	flag.StringVar(&options.OutputPath, "output", options.OutputPath, "path to the generated seed manifest json")
	flag.Parse()

	if err := database.InitWithOptions(database.Options{
		Type:        database.DBType(*dbType),
		DatabaseURL: *databaseURL,
		SQLitePath:  *sqlitePath,
	}); err != nil {
		log.Fatalf("init database: %v", err)
	}
	defer database.Close()

	result, err := seed.New(options).Run()
	if err != nil {
		log.Fatalf("perf seed failed: %v", err)
	}

	log.Printf("perf seed completed: users=%d channels=%d manifest=%s", result.UsersCreated, result.ChannelsCreated, result.ManifestPath)
}
