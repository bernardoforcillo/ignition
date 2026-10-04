package jobstest_test

import (
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/jobs"
	"github.com/bernardoforcillo/ignition/go-packages/jobs/jobstest"
)

func TestStore_Contract(t *testing.T) {
	jobstest.TestStore(t, func(*testing.T) jobs.Store { return jobstest.NewStore() })
}
