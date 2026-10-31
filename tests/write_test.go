package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/lang"
)

func TestWrite_Validation(t *testing.T) {
	tmp := t.TempDir()
	err := deploy.Write(tmp, deploy.Plan{})
	if err == nil {
		t.Fatal("expected error on empty plan")
	}
}

func TestWrite_RoundTrip(t *testing.T) {
	plans := []deploy.Plan{
		{
			Name:      "shop",
			Domain:    "app.example.com",
			Target:    "webtyp.com/selfupdate.Server",
			CI:        "webtyp.com/gorelease.Local",
			Vars:      []deploy.Var{{Key: "APP_ENV", Value: "production"}},
			Secrets: []deploy.Secret{
				{
					Key:    "TOKEN",
					What:   "API token",
					Source: deploy.Typed,
					Guide:  []lang.Text{"Go to dashboard", "Create token"},
					To:     []deploy.Place{deploy.ToRuntime},
				},
				{
					Key:    "KEY",
					What:   "Random key",
					Source: deploy.Random,
					To:     []deploy.Place{deploy.ToRuntime, deploy.ToLocal},
				},
			},
		},
		{
			Name:      "worker-app",
			Domain:    "worker.example.com",
			Target:    "webtyp.com/goflare.Worker",
			CI:        "webtyp.com/github.Actions",
			Databases: []deploy.Database{{Binding: "DB", Name: "my-db"}},
			Buckets:   []deploy.Bucket{{Binding: "UPLOADS", Name: "my-bucket"}},
		},
	}

	for _, p := range plans {
		t.Run(p.Name, func(t *testing.T) {
			tmp := t.TempDir()
			if err := deploy.Write(tmp, p); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			// Read back using deploy.Scan
			scanned, err := deploy.Scan(tmp)
			if err != nil {
				content, _ := os.ReadFile(filepath.Join(tmp, deploy.DefaultFile))
				t.Fatalf("Scan failed: %v\nFile content:\n%s", err, string(content))
			}

			if !reflect.DeepEqual(scanned, p) {
				t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", scanned, p)
			}
		})
	}
}
