package requirement

import (
	"github.com/CJhuochai/project-brain/internal/storage"
	"strings"
	"testing"
)

func TestKeywordDedupPrecedesBudget(t *testing.T) {
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.UpsertFile("a", "Common.java", []byte("Common"))
	db.UpsertFile("b", "Important.java", []byte("Important"))
	r, err := Analyze(db, strings.Repeat("Common ", 65)+"Important")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Keywords) != 2 || len(r.Repositories) != 2 {
		t.Fatalf("keywords=%#v repositories=%#v", r.Keywords, r.Repositories)
	}
	r2, err := Analyze(db, "Common common Common Important")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Evidence) != len(r2.Evidence) {
		t.Fatalf("duplicate keywords inflated evidence: %d %d", len(r.Evidence), len(r2.Evidence))
	}
}
