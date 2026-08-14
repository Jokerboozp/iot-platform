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
	files := []string{"compose.yaml", filepath.Join("deploy", "k8s", "platform.yaml"), filepath.Join("deploy", "k8s", "backup-cronjob.yaml"), filepath.Join("ops", "prometheus", "prometheus.yml"), filepath.Join("ops", "prometheus", "alerts.yml"), filepath.Join("ops", "loki", "loki.yml")}
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
