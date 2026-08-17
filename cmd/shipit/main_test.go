package main

import (
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
				p.Package.PackageType = "container"
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
				p.Package.PackageType = "container"
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
				p.Package.PackageType = "container"
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
