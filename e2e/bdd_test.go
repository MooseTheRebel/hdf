//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
)

// defaultTags skips scenarios that are still being written (@wip) and ones
// that document a known bug (@bug) until it's fixed.
const defaultTags = "~@wip && ~@bug"

// TestFeatures runs the plain-language scenarios in features/, each against
// its own simulated set of machines, then checks them against the state graph
// in model_test.go: every transition they caused must be in the model, and
// (on a full run) every model edge must have been walked.
//
// Environment:
//
//	HDF_BDD_TAGS    tag expression to run a subset (disables the unwalked-edge check)
//	HDF_BDD_FORMAT  godog formatter (default "pretty")
//	HDF_BDD_REPORT  where to write the state-graph report
//	                (default ../build/coverage/state-graph.md)
func TestFeatures(t *testing.T) {
	tags, subset := os.LookupEnv("HDF_BDD_TAGS")
	if !subset {
		tags = defaultTags
	}
	format := os.Getenv("HDF_BDD_FORMAT")
	if format == "" {
		format = "pretty"
	}
	reportPath := os.Getenv("HDF_BDD_REPORT")
	if reportPath == "" {
		reportPath = filepath.Join("..", "build", "coverage", "state-graph.md")
	}

	rec := newRecorder()
	suite := godog.TestSuite{
		Name: "hdf",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
				var tagNames []string
				for _, tag := range s.Tags {
					tagNames = append(tagNames, tag.Name)
				}
				w, err := newWorld(s.Name, tagNames, rec)
				if err != nil {
					return ctx, err
				}
				return context.WithValue(ctx, worldKey{}, w), nil
			})
			sc.After(func(ctx context.Context, s *godog.Scenario, err error) (context.Context, error) {
				w, ok := ctx.Value(worldKey{}).(*world)
				if !ok {
					return ctx, nil
				}
				rec.journey(w.scenario, w.steps)
				return ctx, w.cleanup()
			})
			registerSteps(sc)
		},
		Options: &godog.Options{
			Format:      format,
			Paths:       []string{"features"},
			Tags:        tags,
			Strict:      true,
			Concurrency: 4,
			TestingT:    t,
		},
	}
	if suite.Run() != 0 {
		t.Error("some scenarios failed")
	}

	if err := rec.report(reportPath); err != nil {
		t.Errorf("writing state-graph report: %v", err)
	} else {
		t.Logf("state-graph report: %s", reportPath)
	}
	for _, u := range rec.unexpected() {
		t.Errorf("transition not in the model (e2e/model_test.go): %s", u)
	}
	if subset {
		return
	}
	for _, e := range rec.unwalked() {
		t.Errorf("model edge no scenario walks: %s", e)
	}
}
