// Package ids gera identificadores aleatórios e implementa o relógio do sistema.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Random gera ids de 128 bits em base32 minúsculo (seguros para caminhos e URLs).
type Random struct{}

var _ ports.IDGenerator = Random{}

func (Random) NewJobID() domain.JobID       { return domain.JobID("vj_" + token()) }
func (Random) NewRenderID() domain.RenderID { return domain.RenderID("rn_" + token()) }

func token() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read nunca falha (Go >= 1.24)
	return strings.ToLower(encoding.EncodeToString(b[:]))
}

// SystemClock usa o relógio real em UTC, com precisão de microssegundos (igual ao Postgres).
type SystemClock struct{}

var _ ports.Clock = SystemClock{}

func (SystemClock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
