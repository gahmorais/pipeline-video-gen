package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/storage"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/worker"
)

type refs map[string]struct{}

func (r refs) ReferencedOutputKeys(context.Context) (map[string]struct{}, error) { return r, nil }

func TestMediaCleanupWithLocalFS(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	write := func(key string, age time.Duration) {
		p := filepath.Join(root, filepath.FromSlash(key))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("12345"), 0o644)
		_ = os.Chtimes(p, now.Add(-age), now.Add(-age))
	}
	write("job1/atual.mp4", 30*24*time.Hour)          // referenciado: nunca apaga
	write("job1/antigo.mp4", 10*24*time.Hour)         // órfão velho: apaga
	write("job1/recente.mp4", time.Hour)              // órfão recente: mantém (pode ser render em registro)
	write("job2/r.mp4.partial-abc.mp4", 48*time.Hour) // parcial velho: apaga
	write("job2/r2.mp4.partial-def.mp4", time.Minute) // parcial em andamento: mantém
	write("job3/sozinho.mp4", 10*24*time.Hour)        // órfão velho: apaga (e remove dir vazio)

	fs, _ := storage.NewLocalFS(root)
	c := worker.MediaCleanup{
		Files:        fs,
		References:   refs{"job1/atual.mp4": {}},
		PartialAfter: 24 * time.Hour,
		OrphanAfter:  7 * 24 * time.Hour,
		Logger:       discard,
	}
	res, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 3 || res.FreedBytes != 15 {
		t.Fatalf("resultado: %+v", res)
	}
	entries, _ := fs.List(context.Background())
	var keys []string
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	want := []string{"job1/atual.mp4", "job1/recente.mp4", "job2/r2.mp4.partial-def.mp4"}
	if len(keys) != len(want) {
		t.Fatalf("restantes: %v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("restantes: %v", keys)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "job3")); !os.IsNotExist(err) {
		t.Fatal("diretório vazio deveria ser removido")
	}
	if err := fs.Remove(context.Background(), "../fora.mp4"); err == nil {
		t.Fatal("remove fora da raiz deve falhar")
	}
}
