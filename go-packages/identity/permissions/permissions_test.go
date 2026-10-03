package permissions_test

import (
	"slices"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"
)

func TestStatements_FileHasReadAndWrite(t *testing.T) {
	got := permissions.Statements()[permissions.ResourceFile]
	if !slices.Contains(got, permissions.ActionRead) || !slices.Contains(got, permissions.ActionWrite) {
		t.Errorf("file actions = %v, want read and write", got)
	}
	if permissions.NewAccess() == nil {
		t.Fatal("the access engine must build with the file resource declared")
	}
}
