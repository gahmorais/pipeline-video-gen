// Package schema implementa ports.TemplateCatalog com os JSON Schemas gerados
// a partir do Zod (packages/video-schema) e embutidos no binário.
package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Cópia gerada por `pnpm --filter @videogen/video-schema generate`. Não editar à mão.
//
//go:embed schemas/*.schema.json
var embedded embed.FS

var fileNamePattern = regexp.MustCompile(`^([A-Za-z0-9_-]+)\.v([0-9]+)\.schema\.json$`)

type entry struct {
	raw      []byte
	compiled *jsonschema.Schema
}

// Catalog é imutável após a construção e seguro para uso concorrente.
type Catalog struct {
	entries map[domain.TemplateRef]entry
	latest  map[string]domain.TemplateRef
}

var _ ports.TemplateCatalog = (*Catalog)(nil)

// NewEmbeddedCatalog carrega e compila todos os schemas embutidos.
// Falha no startup se algum schema for inválido (fail fast).
func NewEmbeddedCatalog() (*Catalog, error) {
	sub, err := fs.Sub(embedded, "schemas")
	if err != nil {
		return nil, err
	}
	return NewCatalogFromFS(sub)
}

// NewCatalogFromFS permite testes com outros conjuntos de schemas.
func NewCatalogFromFS(fsys fs.FS) (*Catalog, error) {
	files, err := fs.Glob(fsys, "*.schema.json")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("nenhum schema de template encontrado")
	}

	c := &Catalog{entries: map[domain.TemplateRef]entry{}, latest: map[string]domain.TemplateRef{}}
	for _, name := range files {
		m := fileNamePattern.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("nome de schema fora do padrão <id>.v<n>.schema.json: %s", name)
		}
		version, _ := strconv.Atoi(m[2])
		ref, err := domain.NewTemplateRef(m[1], version)
		if err != nil {
			return nil, err
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		compiled, err := compile(name, raw)
		if err != nil {
			return nil, fmt.Errorf("compilando %s: %w", name, err)
		}
		c.entries[ref] = entry{raw: raw, compiled: compiled}
		if cur, ok := c.latest[ref.ID]; !ok || ref.Version > cur.Version {
			c.latest[ref.ID] = ref
		}
	}
	return c, nil
}

func compile(name string, raw []byte) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	// Não buscar referências remotas: todo schema deve ser autocontido.
	compiler.LoadURL = func(s string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("referência externa não permitida: %s", s)
	}
	url := "mem://schemas/" + name
	if err := compiler.AddResource(url, bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return compiler.Compile(url)
}

func (c *Catalog) Latest(templateID string) (domain.TemplateRef, bool) {
	ref, ok := c.latest[templateID]
	return ref, ok
}

func (c *Catalog) JSONSchema(ref domain.TemplateRef) ([]byte, bool) {
	e, ok := c.entries[ref]
	if !ok {
		return nil, false
	}
	return bytes.Clone(e.raw), true
}

// Validate devolve problemas com caminhos no mesmo formato do Zod ("scenes.0.text").
func (c *Catalog) Validate(ref domain.TemplateRef, storyboard []byte) ([]ports.ValidationIssue, error) {
	e, ok := c.entries[ref]
	if !ok {
		return nil, fmt.Errorf("template %s não suportado", ref)
	}

	dec := json.NewDecoder(bytes.NewReader(storyboard))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return []ports.ValidationIssue{{Path: "", Message: "JSON inválido: " + err.Error()}}, nil
	}
	if dec.More() {
		return []ports.ValidationIssue{{Path: "", Message: "conteúdo extra após o objeto JSON"}}, nil
	}

	err := e.compiled.Validate(doc)
	if err == nil {
		return nil, nil
	}
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return nil, fmt.Errorf("validando storyboard: %w", err)
	}
	return collectIssues(verr), nil
}

// discriminator é a propriedade que identifica o ramo de uma união (z.discriminatedUnion).
const discriminator = "type"

// collectIssues achata a árvore de erros mantendo só as folhas (causas concretas).
// Em uniões (oneOf), mantém apenas o ramo cujo discriminador casou, evitando ruído
// como "type must be product" para uma cena "title".
func collectIssues(root *jsonschema.ValidationError) []ports.ValidationIssue {
	seen := map[ports.ValidationIssue]bool{}
	var out []ports.ValidationIssue
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			issue := ports.ValidationIssue{Path: pointerToPath(e.InstanceLocation), Message: e.Message}
			if !seen[issue] {
				seen[issue] = true
				out = append(out, issue)
			}
			return
		}
		causes := e.Causes
		if strings.HasSuffix(e.KeywordLocation, "/oneOf") {
			causes = matchingBranches(e)
		}
		for _, cause := range causes {
			walk(cause)
		}
	}
	walk(root)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// matchingBranches devolve os ramos do oneOf em que o discriminador NÃO falhou.
// Se nenhum ramo casar (tipo desconhecido), devolve só os erros do discriminador.
func matchingBranches(oneOf *jsonschema.ValidationError) []*jsonschema.ValidationError {
	discLoc := oneOf.InstanceLocation + "/" + discriminator
	var matching, discErrors []*jsonschema.ValidationError
	for _, branch := range oneOf.Causes {
		if leaf := findLeafAt(branch, discLoc); leaf != nil {
			discErrors = append(discErrors, leaf)
			continue
		}
		matching = append(matching, branch)
	}
	if len(matching) > 0 {
		return matching
	}
	return discErrors
}

func findLeafAt(e *jsonschema.ValidationError, instanceLoc string) *jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		if e.InstanceLocation == instanceLoc {
			return e
		}
		return nil
	}
	for _, c := range e.Causes {
		if found := findLeafAt(c, instanceLoc); found != nil {
			return found
		}
	}
	return nil
}

// pointerToPath converte JSON Pointer ("/scenes/0/text") para "scenes.0.text".
func pointerToPath(pointer string) string {
	if pointer == "" || pointer == "/" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i, p := range parts {
		parts[i] = strings.NewReplacer("~1", "/", "~0", "~").Replace(p)
	}
	return strings.Join(parts, ".")
}
