package schema_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/schema"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

// Fixtures compartilhadas com os testes TypeScript (mesmo contrato nas duas linguagens).
var fixturesDir = filepath.Join("..", "..", "..", "..", "..", "packages", "video-schema", "fixtures")

var promoV1 = domain.TemplateRef{ID: "promo", Version: 1}

func newCatalog(t *testing.T) *schema.Catalog {
	t.Helper()
	c, err := schema.NewEmbeddedCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEmbeddedSchemaIsInSyncWithCanonical(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join(fixturesDir, "..", "schemas", "promo.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, ok := newCatalog(t).JSONSchema(promoV1)
	if !ok {
		t.Fatal("schema promo@1 ausente")
	}
	if !bytes.Equal(canonical, embedded) {
		t.Fatal("cópia embutida diverge de packages/video-schema/schemas — rode `pnpm --filter @videogen/video-schema generate`")
	}
}

func TestLatest(t *testing.T) {
	c := newCatalog(t)
	if ref, ok := c.Latest("promo"); !ok || ref != promoV1 {
		t.Fatalf("Latest(promo) = %v, %v", ref, ok)
	}
	if _, ok := c.Latest("nope"); ok {
		t.Fatal("template desconhecido não deve existir")
	}
}

func TestLatestPicksHighestVersion(t *testing.T) {
	minimal := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	c, err := schema.NewCatalogFromFS(fstest.MapFS{
		"promo.v1.schema.json":  {Data: minimal},
		"promo.v10.schema.json": {Data: minimal},
		"promo.v2.schema.json":  {Data: minimal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref, _ := c.Latest("promo"); ref.Version != 10 {
		t.Fatalf("esperado v10 (ordem numérica), obtido %v", ref)
	}
}

func TestCatalogRejectsBadSchemas(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"vazio":               {},
		"nome fora do padrão": {"promo.schema.json": {Data: []byte(`{}`)}},
		"JSON inválido":       {"promo.v1.schema.json": {Data: []byte(`{`)}},
		"ref externa":         {"promo.v1.schema.json": {Data: []byte(`{"$ref":"https://evil.example/s.json"}`)}},
	}
	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := schema.NewCatalogFromFS(fsys); err == nil {
				t.Fatal("esperado erro")
			}
		})
	}
}

func TestValidFixtures(t *testing.T) {
	c := newCatalog(t)
	files, _ := filepath.Glob(filepath.Join(fixturesDir, "valid", "*.json"))
	if len(files) == 0 {
		t.Fatal("nenhuma fixture válida encontrada")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, _ := os.ReadFile(f)
			issues, err := c.Validate(promoV1, raw)
			if err != nil || len(issues) != 0 {
				t.Fatalf("esperado válido, obtido issues=%v err=%v", issues, err)
			}
		})
	}
}

// TestInvalidFixtures garante paridade com o Zod: os mesmos casos são rejeitados,
// e o caminho apontado pelo Zod aparece (ou é ancestral/descendente) entre os do Go.
func TestInvalidFixtures(t *testing.T) {
	c := newCatalog(t)
	raw, err := os.ReadFile(filepath.Join(fixturesDir, "invalid", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name      string          `json:"name"`
		ErrorPath string          `json:"errorPath"`
		Input     json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			issues, err := c.Validate(promoV1, tc.Input)
			if err != nil {
				t.Fatal(err)
			}
			if len(issues) == 0 {
				t.Fatal("esperado inválido")
			}
			for _, i := range issues {
				if related(i.Path, tc.ErrorPath) {
					return
				}
			}
			t.Fatalf("nenhum issue relacionado a %q: %+v", tc.ErrorPath, issues)
		})
	}
}

func related(a, b string) bool {
	if a == b || a == "" || b == "" {
		return true
	}
	return strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
}

func TestValidateEdgeCases(t *testing.T) {
	c := newCatalog(t)
	tests := []struct {
		name  string
		input string
	}{
		{"não-JSON", `oops`},
		{"array", `[]`},
		{"lixo após objeto", `{"schemaVersion":1} {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := c.Validate(promoV1, []byte(tt.input))
			if err != nil || len(issues) == 0 {
				t.Fatalf("esperado issues, obtido %v %v", issues, err)
			}
		})
	}
	if _, err := c.Validate(domain.TemplateRef{ID: "promo", Version: 9}, []byte(`{}`)); err == nil {
		t.Fatal("versão não suportada deve retornar erro")
	}
}

func TestUnionIssuesFollowDiscriminator(t *testing.T) {
	c := newCatalog(t)
	tests := []struct {
		name  string
		scene string
		want  []string // "path: trecho da mensagem"
	}{
		{
			name:  "cena title inválida não mostra erros dos outros ramos",
			scene: `{"type":"title","text":"","durationSec":9}`,
			want:  []string{"scenes.0.durationSec: <= 8", "scenes.0.text: >= 1"},
		},
		{
			name:  "tipo desconhecido reporta só o discriminador",
			scene: `{"type":"video","durationSec":2}`,
			want:  []string{"scenes.0.type: title", "scenes.0.type: product", "scenes.0.type: cta"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := `{"schemaVersion":1,"brand":{"primary":"#112233","logoUrl":"a"},"scenes":[` + tt.scene + `]}`
			issues, err := c.Validate(promoV1, []byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			if len(issues) != len(tt.want) {
				t.Fatalf("esperado %d issues, obtido %+v", len(tt.want), issues)
			}
			for _, w := range tt.want {
				path, fragment, _ := strings.Cut(w, ": ")
				found := false
				for _, i := range issues {
					if i.Path == path && strings.Contains(i.Message, fragment) {
						found = true
					}
				}
				if !found {
					t.Fatalf("issue %q não encontrado em %+v", w, issues)
				}
			}
		})
	}
}

func TestJSONSchemaReturnsCopy(t *testing.T) {
	c := newCatalog(t)
	a, _ := c.JSONSchema(promoV1)
	a[0] = 'X'
	b, _ := c.JSONSchema(promoV1)
	if b[0] == 'X' {
		t.Fatal("JSONSchema expôs buffer interno")
	}
}
