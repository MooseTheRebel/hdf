//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
//	HDF_BDD_TAGS    tag expression to run a subset (disables the unwalked-edge
//	                check); defaultTags' exclusions still apply unless named
//	HDF_BDD_FORMAT  godog formatter (default "pretty")
//	HDF_BDD_REPORT  where to write the state-graph report (default
//	                ../build/coverage/state-graph.md; a subset writes none
//	                unless this is set)
func TestFeatures(t *testing.T) {
	tags, subset, reportPath := bddOptions(os.LookupEnv)
	format := os.Getenv("HDF_BDD_FORMAT")
	if format == "" {
		format = "pretty"
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

	if reportPath != "" {
		if err := rec.report(reportPath); err != nil {
			t.Errorf("writing state-graph report: %v", err)
		} else {
			t.Logf("state-graph report: %s", reportPath)
		}
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

// bddOptions reads TestFeatures' settings from the environment via lookup:
// the tag expression to run, whether that's a subset, and where to write the
// state-graph report ("" for none).
//
// A subset keeps each of defaultTags' exclusions unless it names that tag
// itself, so "~@wip" still skips @bug while "@bug" runs it. A subset writes a
// report only to an explicit HDF_BDD_REPORT, never over the full run's.
func bddOptions(lookup func(string) (string, bool)) (tags string, subset bool, report string) {
	report, _ = lookup("HDF_BDD_REPORT")
	tags, _ = lookup("HDF_BDD_TAGS")
	if tags == "" {
		if report == "" {
			report = filepath.Join("..", "build", "coverage", "state-graph.md")
		}
		return defaultTags, false, report
	}
	named := map[string]bool{}
	for _, and := range strings.Split(tags, "&&") {
		for _, tag := range strings.Split(and, ",") {
			named[strings.TrimPrefix(strings.TrimSpace(tag), "~")] = true
		}
	}
	for _, exclude := range strings.Split(defaultTags, "&&") {
		exclude = strings.TrimSpace(exclude)
		if !named[strings.TrimPrefix(exclude, "~")] {
			tags += " && " + exclude
		}
	}
	return tags, true, report
}
