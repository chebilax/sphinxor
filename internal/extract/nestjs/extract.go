// Package nestjs extracts Sphinxor's intermediate model
// (internal/model) from a NestJS project, per
// docs/decisions/0001-target-framework-choice.md.
package nestjs

import (
	"errors"

	"github.com/chebilax/sphinxor/internal/model"
)

var errNotImplemented = errors.New("nestjs: extraction not implemented yet")

// Extract walks the NestJS project rooted at dir and builds the
// intermediate model: controllers, endpoints, guard applications, role
// declarations, and role references.
//
// Not yet implemented. This is the next phase of work, after the
// structural skeleton is confirmed.
func Extract(dir string) (*model.Model, error) {
	return nil, errNotImplemented
}
