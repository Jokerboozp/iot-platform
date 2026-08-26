package deploycheck

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDeploymentYAMLParses(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	files := []string{"compose.yaml", "compose.offline.yaml", filepath.Join("deploy", "k8s", "platform.yaml"), filepath.Join("deploy", "k8s", "backup-cronjob.yaml"), filepath.Join("ops", "prometheus", "prometheus.yml"), filepath.Join("ops", "prometheus", "alerts.yml"), filepath.Join("ops", "loki", "loki.yml")}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			decoder := yaml.NewDecoder(f)
			documents := 0
			for {
				var doc any
				err = decoder.Decode(&doc)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				documents++
			}
			if documents == 0 {
				t.Fatal("no YAML documents")
			}
		})
	}
}

func TestBackupServiceIsOptInComposeProfile(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	content, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Services map[string]struct {
			Profiles []string `yaml:"profiles"`
			Restart  string   `yaml:"restart"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	backup, ok := document.Services["backup-service"]
	if !ok {
		t.Fatal("compose.yaml must define backup-service")
	}
	profileFound := false
	for _, profile := range backup.Profiles {
		if profile == "backup" {
			profileFound = true
			break
		}
	}
	if !profileFound {
		t.Fatalf("backup-service must be guarded by the backup profile, got profiles %v", backup.Profiles)
	}
	if backup.Restart != "no" {
		t.Fatalf("backup-service must require manual restart, got restart policy %q", backup.Restart)
	}
}
