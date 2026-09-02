package main

import (
	"encoding/json"
	"testing"
)

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name    string
		payload WebhookPayload
		want    bool
	}{
		{
			name: "matches all criteria",
			payload: func() WebhookPayload {
				var p WebhookPayload
				p.Action = "published"
				p.Package.PackageType = "CONTAINER"
				p.Package.PackageVersion.ContainerMetadata.Tag.Name = "latest"
				return p
			}(),
			want: true,
		},
		{
			name: "wrong action",
			payload: func() WebhookPayload {
				var p WebhookPayload
				p.Action = "updated"
				p.Package.PackageType = "CONTAINER"
				p.Package.PackageVersion.ContainerMetadata.Tag.Name = "latest"
				return p
			}(),
			want: false,
		},
		{
			name: "wrong package type",
			payload: func() WebhookPayload {
				var p WebhookPayload
				p.Action = "published"
				p.Package.PackageType = "npm"
				p.Package.PackageVersion.ContainerMetadata.Tag.Name = "latest"
				return p
			}(),
			want: false,
		},
		{
			name: "wrong tag",
			payload: func() WebhookPayload {
				var p WebhookPayload
				p.Action = "published"
				p.Package.PackageType = "CONTAINER"
				p.Package.PackageVersion.ContainerMetadata.Tag.Name = "v1.2.3"
				return p
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesFilter(&tt.payload); got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildWhitelistSet(t *testing.T) {
	repos := []string{"org1/repo1", "org2/repo2", "org3/repo3"}
	wl := buildWhitelistSet(repos)

	if len(wl) != len(repos) {
		t.Fatalf("expected %d entries, got %d", len(repos), len(wl))
	}
	for _, r := range repos {
		if _, ok := wl[r]; !ok {
			t.Errorf("expected %q in whitelist set", r)
		}
	}
}

func TestBuildWhitelistSetEmpty(t *testing.T) {
	wl := buildWhitelistSet(nil)
	if len(wl) != 0 {
		t.Errorf("expected empty set, got %d entries", len(wl))
	}
}

func TestRepositoryName(t *testing.T) {
	tests := []struct {
		fullName string
		want     string
	}{
		{fullName: "its-the-vibe/SlashVibeRepo", want: "SlashVibeRepo"},
		{fullName: "SlashVibeRepo", want: "SlashVibeRepo"},
	}

	for _, tt := range tests {
		t.Run(tt.fullName, func(t *testing.T) {
			if got := repositoryName(tt.fullName); got != tt.want {
				t.Errorf("repositoryName(%q) = %q, want %q", tt.fullName, got, tt.want)
			}
		})
	}
}

func TestMatchesCustomFilter(t *testing.T) {
	tests := []struct {
		name    string
		payload CustomImagePayload
		want    bool
	}{
		{
			name:    "matches all criteria",
			payload: CustomImagePayload{Event: customEventName, Ref: customRefName},
			want:    true,
		},
		{
			name:    "wrong event",
			payload: CustomImagePayload{Event: "other_event", Ref: customRefName},
			want:    false,
		},
		{
			name:    "wrong ref",
			payload: CustomImagePayload{Event: customEventName, Ref: "feature-branch"},
			want:    false,
		},
		{
			name:    "both wrong",
			payload: CustomImagePayload{Event: "other_event", Ref: "feature-branch"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesCustomFilter(&tt.payload); got != tt.want {
				t.Errorf("matchesCustomFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCustomImagePayloadDecodesStringTags(t *testing.T) {
	rawPayload := `{
		"event": "image_pushed",
		"repository": "its-the-vibe/OrderlyQueue",
		"ref": "main",
		"sha": "be877120899dd0f86e7db57d8f545a47e8a046b4",
		"image": "ghcr.io/its-the-vibe/OrderlyQueue",
		"tags": "ghcr.io/its-the-vibe/orderlyqueue:main\nghcr.io/its-the-vibe/orderlyqueue:latest\nghcr.io/its-the-vibe/orderlyqueue:sha-be87712"
	}`

	var payload CustomImagePayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	wantTags := "ghcr.io/its-the-vibe/orderlyqueue:main\nghcr.io/its-the-vibe/orderlyqueue:latest\nghcr.io/its-the-vibe/orderlyqueue:sha-be87712"
	if payload.Tags != wantTags {
		t.Errorf("Tags = %q, want %q", payload.Tags, wantTags)
	}
}

func TestBuildDeployMessageWithoutMetadata(t *testing.T) {
	deploy := buildDeployMessage("its-the-vibe/TurnItOffAndOnAgain", "deploy-queue", "")
	data, err := json.Marshal(deploy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got := string(data)
	want := `{"restart":"TurnItOffAndOnAgain","target-queue":"deploy-queue"}`
	if got != want {
		t.Errorf("deploy message = %s, want %s", got, want)
	}
}

func TestBuildDeployMessageWithGitCommitSHAMetadata(t *testing.T) {
	deploy := buildDeployMessage(
		"its-the-vibe/TurnItOffAndOnAgain",
		"deploy-queue",
		"77e47ec39a0230c295a7fbbb1d4f12139ed8e586",
	)
	data, err := json.Marshal(deploy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got := string(data)
	want := `{"restart":"TurnItOffAndOnAgain","target-queue":"deploy-queue","metadata":{"git_commit_sha":"77e47ec39a0230c295a7fbbb1d4f12139ed8e586"}}`
	if got != want {
		t.Errorf("deploy message = %s, want %s", got, want)
	}
}
