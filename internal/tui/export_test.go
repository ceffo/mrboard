// export_test.go exposes internal Model state for white-box tests.
// This file is only compiled when running `go test`.
package tui

import "github.com/ceffo/mrboard/internal/domain"

func (m Model) AllMRs() []domain.MergeRequest { return m.allMRs }
func (m Model) MyView() bool                  { return m.viewMode == domain.ViewMine }
func (m Model) SortFieldKey() string          { return m.sortField.stateKey() }
func (m Model) SortDesc() bool                { return m.sortDesc }
func (m Model) ShowDetail() bool              { return m.showDetail }
func (m Model) State() appState               { return m.state }
func (m Model) ErrMsg() string                { return m.errMsg }
func (m Model) Errors() []error               { return m.errors }

// StateLoading / StateBoard / StateError are exported sentinels for test assertions.
const (
	StateLoading = stateLoading
	StateBoard   = stateBoard
	StateError   = stateError
)
