package database

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const postgresDumpTimeout = 5 * time.Minute

type postgresConnectionInfo struct {
	Database string
	Host     string
	Password string
	Port     string
	User     string
}

func DumpPostgresDatabase(ctx context.Context, options Options) ([]byte, error) {
	normalized, err := options.Normalize()
	if err != nil {
		return nil, err
	}
	if normalized.Type != DBTypePostgres {
		return nil, fmt.Errorf("dump only supports postgres")
	}

	dumpCtx, cancel := context.WithTimeout(ctx, postgresDumpTimeout)
	defer cancel()

	return runPostgresCommand(dumpCtx, normalized.DatabaseURL, nil, postgresCommandModeDump)
}

func RestorePostgresDatabase(ctx context.Context, options Options, dumpContent []byte) error {
	normalized, err := options.Normalize()
	if err != nil {
		return err
	}
	if normalized.Type != DBTypePostgres {
		return fmt.Errorf("restore only supports postgres")
	}

	restoreCtx, cancel := context.WithTimeout(ctx, postgresDumpTimeout)
	defer cancel()

	_, err = runPostgresCommand(restoreCtx, normalized.DatabaseURL, dumpContent, postgresCommandModeRestore)
	return err
}

type postgresCommandMode string

const (
	postgresCommandModeDump    postgresCommandMode = "dump"
	postgresCommandModeRestore postgresCommandMode = "restore"
)

func runPostgresCommand(ctx context.Context, databaseURL string, stdin []byte, mode postgresCommandMode) ([]byte, error) {
	if commandName, err := exec.LookPath(commandForMode(mode)); err == nil {
		return runHostPostgresCommand(ctx, commandName, databaseURL, stdin, mode)
	}

	if mode == postgresCommandModeDump {
		return runQueryPostgresDump(ctx, databaseURL)
	}

	connectionInfo, err := parsePostgresConnectionInfo(databaseURL)
	if err != nil {
		return nil, err
	}
	if !canUseDockerPostgresFallback(connectionInfo.Host) {
		return nil, fmt.Errorf("%s not found in PATH and database host appears remote, cannot fallback to docker safely", commandForMode(mode))
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("%s not found in PATH and docker not found in PATH for local fallback", commandForMode(mode))
	}

	containerName, err := findLocalPostgresContainer(ctx)
	if err != nil {
		return nil, err
	}

	return runDockerPostgresCommand(ctx, containerName, connectionInfo, stdin, mode)
}

func commandForMode(mode postgresCommandMode) string {
	if mode == postgresCommandModeRestore {
		return "psql"
	}
	return "pg_dump"
}

func runHostPostgresCommand(ctx context.Context, commandName, databaseURL string, stdin []byte, mode postgresCommandMode) ([]byte, error) {
	args := []string{fmt.Sprintf("--dbname=%s", databaseURL)}
	if mode == postgresCommandModeDump {
		args = append(args, "--format=plain", "--clean", "--if-exists", "--no-owner", "--no-privileges")
	} else {
		args = append(args, "-v", "ON_ERROR_STOP=1")
	}

	command := exec.CommandContext(ctx, commandName, args...)
	if len(stdin) > 0 {
		command.Stdin = bytes.NewReader(stdin)
	}

	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", commandName, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func runDockerPostgresCommand(ctx context.Context, containerName string, info postgresConnectionInfo, stdin []byte, mode postgresCommandMode) ([]byte, error) {
	args := []string{"exec", "-i", "-e", "PGPASSWORD=" + info.Password, containerName, commandForMode(mode), "-h", "127.0.0.1", "-p", info.Port, "-U", info.User, "-d", info.Database}
	if mode == postgresCommandModeDump {
		args = append(args, "--format=plain", "--clean", "--if-exists", "--no-owner", "--no-privileges")
	} else {
		args = append(args, "-v", "ON_ERROR_STOP=1")
	}

	command := exec.CommandContext(ctx, "docker", args...)
	if len(stdin) > 0 {
		command.Stdin = bytes.NewReader(stdin)
	}

	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %s failed: %w: %s", commandForMode(mode), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func parsePostgresConnectionInfo(databaseURL string) (postgresConnectionInfo, error) {
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return postgresConnectionInfo{}, err
	}

	password, _ := parsedURL.User.Password()
	port := parsedURL.Port()
	if port == "" {
		port = "5432"
	}

	return postgresConnectionInfo{
		Database: strings.TrimPrefix(parsedURL.Path, "/"),
		Host:     parsedURL.Hostname(),
		Password: password,
		Port:     port,
		User:     parsedURL.User.Username(),
	}, nil
}

func canUseDockerPostgresFallback(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return true
	}

	return false
}

func findLocalPostgresContainer(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to inspect docker containers: %w: %s", err, strings.TrimSpace(string(output)))
	}

	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if strings.Contains(name, "postgres") {
			return name, nil
		}
	}

	return "", fmt.Errorf("no running postgres container found for docker fallback")
}

type postgresDumpColumn struct {
	Default sql.NullString
	Name    string
	Type    string
	NotNull bool
}

type postgresDumpConstraint struct {
	Definition string
	Name       string
}

type postgresDumpTable struct {
	Name              string
	Columns           []postgresDumpColumn
	Indexes           []string
	InlineConstraints []postgresDumpConstraint
	ForeignKeys       []postgresDumpConstraint
}

func runQueryPostgresDump(ctx context.Context, databaseURL string) ([]byte, error) {
	db, err := OpenWithOptions(Options{Type: DBTypePostgres, DatabaseURL: databaseURL})
	if err != nil {
		return nil, err
	}
	defer db.Close()

	tables, err := loadPostgresDumpTables(ctx, db)
	if err != nil {
		return nil, err
	}

	var builder strings.Builder
	builder.WriteString("-- AMP Manager PostgreSQL logical dump\n")
	builder.WriteString("BEGIN;\n")
	builder.WriteString("SET client_encoding = 'UTF8';\n")
	builder.WriteString("SET standard_conforming_strings = on;\n")
	builder.WriteString("SET check_function_bodies = false;\n")
	builder.WriteString("SET client_min_messages = warning;\n")
	builder.WriteString("SET row_security = off;\n\n")

	for index := len(tables) - 1; index >= 0; index-- {
		builder.WriteString("DROP TABLE IF EXISTS ")
		builder.WriteString(postgresTableRef(tables[index].Name))
		builder.WriteString(" CASCADE;\n")
	}
	builder.WriteString("\n")

	for _, table := range tables {
		writePostgresCreateTable(&builder, table)
	}

	for _, table := range tables {
		if err := writePostgresTableData(ctx, db, &builder, table); err != nil {
			return nil, err
		}
	}

	for _, table := range tables {
		for _, indexDef := range table.Indexes {
			statement := strings.TrimSpace(indexDef)
			if statement == "" {
				continue
			}
			builder.WriteString(statement)
			if !strings.HasSuffix(statement, ";") {
				builder.WriteString(";")
			}
			builder.WriteString("\n")
		}
		if len(table.Indexes) > 0 {
			builder.WriteString("\n")
		}
	}

	for _, table := range tables {
		for _, foreignKey := range table.ForeignKeys {
			builder.WriteString("ALTER TABLE ONLY ")
			builder.WriteString(postgresTableRef(table.Name))
			builder.WriteString(" ADD CONSTRAINT ")
			builder.WriteString(postgresQuoteIdentifier(foreignKey.Name))
			builder.WriteString(" ")
			builder.WriteString(foreignKey.Definition)
			builder.WriteString(";\n")
		}
		if len(table.ForeignKeys) > 0 {
			builder.WriteString("\n")
		}
	}

	builder.WriteString("COMMIT;\n")
	return []byte(builder.String()), nil
}

func loadPostgresDumpTables(ctx context.Context, db *sql.DB) ([]postgresDumpTable, error) {
	rows, err := db.QueryContext(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []postgresDumpTable
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		table, err := loadPostgresDumpTable(ctx, db, tableName)
		if err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, rows.Err()
}

func loadPostgresDumpTable(ctx context.Context, db *sql.DB, tableName string) (postgresDumpTable, error) {
	table := postgresDumpTable{Name: tableName}

	columnRows, err := db.QueryContext(ctx, `
		SELECT
			a.attname,
			pg_catalog.format_type(a.atttypid, a.atttypmod) AS formatted_type,
			a.attnotnull,
			pg_get_expr(ad.adbin, ad.adrelid) AS default_expr
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE n.nspname = 'public' AND c.relname = ? AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum ASC
	`, tableName)
	if err != nil {
		return table, err
	}
	defer columnRows.Close()

	for columnRows.Next() {
		var column postgresDumpColumn
		if err := columnRows.Scan(&column.Name, &column.Type, &column.NotNull, &column.Default); err != nil {
			return table, err
		}
		table.Columns = append(table.Columns, column)
	}
	if err := columnRows.Err(); err != nil {
		return table, err
	}

	constraintRows, err := db.QueryContext(ctx, `
		SELECT
			con.conname,
			con.contype,
			pg_get_constraintdef(con.oid, true)
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = rel.relnamespace
		WHERE ns.nspname = 'public' AND rel.relname = ?
		ORDER BY
			CASE con.contype
				WHEN 'p' THEN 0
				WHEN 'u' THEN 1
				WHEN 'c' THEN 2
				WHEN 'f' THEN 3
				ELSE 4
			END,
			con.conname ASC
	`, tableName)
	if err != nil {
		return table, err
	}
	defer constraintRows.Close()

	for constraintRows.Next() {
		var (
			constraint     postgresDumpConstraint
			constraintType string
		)
		if err := constraintRows.Scan(&constraint.Name, &constraintType, &constraint.Definition); err != nil {
			return table, err
		}
		if constraintType == "f" {
			table.ForeignKeys = append(table.ForeignKeys, constraint)
			continue
		}
		table.InlineConstraints = append(table.InlineConstraints, constraint)
	}
	if err := constraintRows.Err(); err != nil {
		return table, err
	}

	indexRows, err := db.QueryContext(ctx, `
		SELECT idx.indexdef
		FROM pg_indexes idx
		JOIN pg_class rel ON rel.relname = idx.tablename
		JOIN pg_namespace ns ON ns.oid = rel.relnamespace
		WHERE idx.schemaname = 'public'
		  AND idx.tablename = ?
		  AND ns.nspname = 'public'
		  AND NOT EXISTS (
			SELECT 1
			FROM pg_constraint con
			JOIN pg_class ci ON ci.oid = con.conindid
			WHERE con.conrelid = rel.oid AND ci.relname = idx.indexname
		  )
		ORDER BY idx.indexname ASC
	`, tableName)
	if err != nil {
		return table, err
	}
	defer indexRows.Close()

	for indexRows.Next() {
		var indexDef string
		if err := indexRows.Scan(&indexDef); err != nil {
			return table, err
		}
		table.Indexes = append(table.Indexes, indexDef)
	}

	return table, indexRows.Err()
}

func writePostgresCreateTable(builder *strings.Builder, table postgresDumpTable) {
	lines := make([]string, 0, len(table.Columns)+len(table.InlineConstraints))
	for _, column := range table.Columns {
		lines = append(lines, "    "+postgresColumnDefinition(column))
	}
	for _, constraint := range table.InlineConstraints {
		lines = append(lines, fmt.Sprintf("    CONSTRAINT %s %s", postgresQuoteIdentifier(constraint.Name), constraint.Definition))
	}

	builder.WriteString("CREATE TABLE ")
	builder.WriteString(postgresTableRef(table.Name))
	builder.WriteString(" (\n")
	builder.WriteString(strings.Join(lines, ",\n"))
	builder.WriteString("\n);\n\n")
}

func writePostgresTableData(ctx context.Context, db *sql.DB, builder *strings.Builder, table postgresDumpTable) error {
	if len(table.Columns) == 0 {
		return nil
	}

	columnRefs := make([]string, 0, len(table.Columns))
	valueExprs := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		columnRef := postgresQuoteIdentifier(column.Name)
		columnRefs = append(columnRefs, columnRef)
		valueExprs = append(valueExprs, fmt.Sprintf("quote_nullable(%s)", columnRef))
	}

	query := fmt.Sprintf(
		"SELECT 'INSERT INTO %s (%s) VALUES (' || array_to_string(ARRAY[%s], ', ') || ');' FROM %s",
		postgresTableRef(table.Name),
		strings.Join(columnRefs, ", "),
		strings.Join(valueExprs, ", "),
		postgresTableRef(table.Name),
	)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	wroteRows := false
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			return err
		}
		builder.WriteString(statement)
		builder.WriteString("\n")
		wroteRows = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if wroteRows {
		builder.WriteString("\n")
	}

	return nil
}

func postgresColumnDefinition(column postgresDumpColumn) string {
	var builder strings.Builder
	builder.WriteString(postgresQuoteIdentifier(column.Name))
	builder.WriteString(" ")
	builder.WriteString(column.Type)
	if column.Default.Valid && strings.TrimSpace(column.Default.String) != "" {
		builder.WriteString(" DEFAULT ")
		builder.WriteString(column.Default.String)
	}
	if column.NotNull {
		builder.WriteString(" NOT NULL")
	}
	return builder.String()
}

func postgresTableRef(tableName string) string {
	return "public." + postgresQuoteIdentifier(tableName)
}

func postgresQuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
