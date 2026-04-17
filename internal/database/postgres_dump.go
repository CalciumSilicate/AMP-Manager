package database

import (
	"bytes"
	"context"
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

	connectionInfo, err := parsePostgresConnectionInfo(databaseURL)
	if err != nil {
		return nil, err
	}
	if !canUseDockerPostgresFallback(connectionInfo.Host) {
		return nil, fmt.Errorf("%s not found in PATH and database host appears remote, cannot fallback to docker safely", commandForMode(mode))
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
