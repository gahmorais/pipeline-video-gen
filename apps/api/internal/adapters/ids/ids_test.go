package ids_test

import (
	"testing"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/ids"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

func TestGeneratedIDsAreValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		j, r := ids.Random{}.NewJobID(), ids.Random{}.NewRenderID()
		if _, err := domain.ParseJobID(string(j)); err != nil {
			t.Fatal(err)
		}
		if _, err := domain.ParseRenderID(string(r)); err != nil {
			t.Fatal(err)
		}
		if seen[string(j)] || seen[string(r)] {
			t.Fatal("id repetido")
		}
		seen[string(j)], seen[string(r)] = true, true
	}
}
