package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	Redis struct {
		Host     string
		Port     int
		Password string
	}
	// Channel is the Redis pub/sub channel to subscribe to.
	Channel string
	// DeployList is the Redis list to push deployment messages onto.
	DeployList string `mapstructure:"deploy_list"`
	// TargetQueue is included in each deployment message as "target-queue".
	TargetQueue string `mapstructure:"target_queue"`
	// WhitelistFile is the path to a file containing allowed "org/repo" entries.
	WhitelistFile string `mapstructure:"whitelist_file"`
}

// WebhookPayload represents the relevant parts of a GitHub package webhook event.
type WebhookPayload struct {
	Action  string `json:"action"`
	Package struct {
		PackageType    string `json:"package_type"`
		PackageVersion struct {
			ContainerMetadata struct {
				Tag struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"container_metadata"`
		} `json:"package_version"`
	} `json:"package"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// DeployMessage is the message published to the deployment Redis list.
type DeployMessage struct {
	Restart     string `json:"restart"`
	TargetQueue string `json:"target-queue"`
}

func loadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/")

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("channel", "github-webhooks")
	viper.SetDefault("deploy_list", "deployments")
	viper.SetDefault("target_queue", "deploy-queue")
	viper.SetDefault("whitelist_file", "whitelist.txt")

	viper.AutomaticEnv()
	viper.BindEnv("redis.password", "REDIS_PASSWORD") //nolint:errcheck

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		log.Println("No config file found, using defaults and environment variables")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	return &cfg, nil
}

// loadWhitelist reads a newline-delimited file of "org/repo" entries and returns
// a set of allowed repository full names.
func loadWhitelist(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening whitelist file %q: %w", path, err)
	}
	defer f.Close()

	wl := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		wl[line] = struct{}{}
	}
	return wl, scanner.Err()
}

// matchesFilter returns true when the payload satisfies the deployment trigger criteria:
//
//	action == "published"
//	AND package.package_type == "container"
//	AND package.package_version.container_metadata.tag.name == "latest"
func matchesFilter(p *WebhookPayload) bool {
	return p.Action == "published" &&
		p.Package.PackageType == "container" &&
		p.Package.PackageVersion.ContainerMetadata.Tag.Name == "latest"
}

// processMessage parses a raw JSON message, applies the filter and whitelist, and
// publishes a deployment command to the configured Redis list if all checks pass.
func processMessage(ctx context.Context, rdb *redis.Client, rawMsg string, whitelist map[string]struct{}, cfg *Config) {
	var payload WebhookPayload
	if err := json.Unmarshal([]byte(rawMsg), &payload); err != nil {
		log.Printf("failed to parse webhook payload: %v", err)
		return
	}

	if !matchesFilter(&payload) {
		log.Printf("skipping event: action=%q package_type=%q tag=%q",
			payload.Action,
			payload.Package.PackageType,
			payload.Package.PackageVersion.ContainerMetadata.Tag.Name,
		)
		return
	}

	repo := payload.Repository.FullName
	if _, ok := whitelist[repo]; !ok {
		log.Printf("repository %q is not in the whitelist, skipping", repo)
		return
	}

	deploy := DeployMessage{
		Restart:     repo,
		TargetQueue: cfg.TargetQueue,
	}
	data, err := json.Marshal(deploy)
	if err != nil {
		log.Printf("failed to marshal deploy message: %v", err)
		return
	}

	if err := rdb.RPush(ctx, cfg.DeployList, string(data)).Err(); err != nil {
		log.Printf("failed to publish deploy message: %v", err)
		return
	}

	log.Printf("queued deployment for %q -> list=%q target-queue=%q", repo, cfg.DeployList, cfg.TargetQueue)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	whitelist, err := loadWhitelist(cfg.WhitelistFile)
	if err != nil {
		log.Fatalf("failed to load whitelist: %v", err)
	}
	log.Printf("loaded %d repositories from whitelist", len(whitelist))

	addr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Redis.Password,
	})
	defer rdb.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pubsub := rdb.Subscribe(ctx, cfg.Channel)
	defer pubsub.Close()

	log.Printf("subscribed to Redis channel %q at %s", cfg.Channel, addr)
	log.Printf("deployment messages will be published to list %q", cfg.DeployList)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case msg, ok := <-ch:
			if !ok {
				log.Println("Redis channel closed, shutting down")
				return
			}
			processMessage(ctx, rdb, msg.Payload, whitelist, cfg)
		}
	}
}
