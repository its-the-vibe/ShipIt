package main

import (
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
	// Whitelist is the list of allowed "org/repo" repositories.
	Whitelist []string `mapstructure:"whitelist"`
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

// buildWhitelistSet converts the slice of repos from config into a fast-lookup set.
func buildWhitelistSet(repos []string) map[string]struct{} {
	wl := make(map[string]struct{}, len(repos))
	for _, r := range repos {
		wl[r] = struct{}{}
	}
	return wl
}

func repositoryName(fullName string) string {
	if ownerSeparator := strings.LastIndexByte(fullName, '/'); ownerSeparator >= 0 {
		return fullName[ownerSeparator+1:]
	}
	return fullName
}

// matchesFilter returns true when the payload satisfies the deployment trigger criteria:
//
//	action == "published"
//	AND package.package_type == "container"
//	AND package.package_version.container_metadata.tag.name == "latest"
func matchesFilter(p *WebhookPayload) bool {
	return p.Action == "published" &&
		p.Package.PackageType == "CONTAINER" &&
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

	fullRepoName := payload.Repository.FullName
	if _, ok := whitelist[fullRepoName]; !ok {
		log.Printf("repository %q is not in the whitelist, skipping", fullRepoName)
		return
	}

	deploy := DeployMessage{
		Restart:     repositoryName(fullRepoName),
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

	log.Printf("queued deployment for %q -> list=%q target-queue=%q", fullRepoName, cfg.DeployList, cfg.TargetQueue)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	whitelist := buildWhitelistSet(cfg.Whitelist)
	log.Printf("loaded %d repositories from whitelist", len(whitelist))

	addr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Redis.Password,
	})
	defer rdb.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to Redis: %v", err)
	}

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
