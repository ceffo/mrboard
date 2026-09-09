package selfupdate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ceffo/mrboard/internal/selfupdate"
)

// TestExecCmd_ShellsOutToBrew asserts command construction only — running it
// would upgrade whichever machine executed the test.
func TestExecCmd_ShellsOutToBrew(t *testing.T) {
	cmd := selfupdate.ExecCmd()

	assert.Equal(t, []string{"sh", "-c", selfupdate.Command}, cmd.Args)
}
