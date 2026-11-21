package normalize

import (
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// The dialect of JUnit XML is its root: testsuites of gotestsum, pytest and jest-junit, or a single testsuite of
// Maven Surefire. JUnit XML declares no version of its own, so the root stands for it.
const (
	_junitSuites = "testsuites"
	_junitSuite  = "testsuite"
	_maxCaseKey  = 2048
	_maxOrdinal  = math.MaxInt16
	// _maxDurationS keeps a duration in milliseconds within int64.
	_maxDurationS = 1e12
)

// parsedTest is a test case of a JUnit report; ordinal counts the same case within the report.
type parsedTest struct {
	key        string
	keyHash    []byte
	ordinal    int
	outcome    string
	durationMS *int64
}

type junitCase struct {
	suite, class, name       string
	duration                 *int64
	failed, errored, skipped bool
}

// parseJUnit reads the test cases of a JUnit XML report as a stream. A case belongs to its nearest suite; its key is
// the suite, the class and the name, and a case without failure, error or skipped passed.
func parseJUnit(r io.Reader, _ parseInput) (*parsedReport, error) {
	dec := xml.NewDecoder(r)
	out := &parsedReport{}
	var (
		suites []string
		tc     *junitCase
	)
	seen := map[string]int{}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if out.formatVersion == "" {
				out.formatVersion = t.Name.Local
				if t.Name.Local != _junitSuites && t.Name.Local != _junitSuite {
					return out, nil
				}
			}
			switch t.Name.Local {
			case _junitSuite:
				suites = append(suites, attr(t, "name"))
			case "testcase":
				if tc != nil {
					return nil, invalidReport("junit_xml: a test case inside a test case")
				}
				tc = &junitCase{class: attr(t, "classname"), name: attr(t, "name"), duration: milliseconds(attr(t, "time"))}
				if len(suites) > 0 {
					tc.suite = suites[len(suites)-1]
				}
			case "failure":
				if tc != nil {
					tc.failed = true
				}
			case "error":
				if tc != nil {
					tc.errored = true
				}
			case "skipped":
				if tc != nil {
					tc.skipped = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case _junitSuite:
				suites = suites[:len(suites)-1]
			case "testcase":
				test, err := tc.test(seen)
				if err != nil {
					return nil, err
				}
				out.tests = append(out.tests, test)
				tc = nil
			}
		}
	}
	if out.formatVersion == "" {
		return nil, invalidReport("junit_xml: no root element")
	}
	return out, nil
}

func (c *junitCase) test(seen map[string]int) (parsedTest, error) {
	if c.name == "" {
		return parsedTest{}, invalidReport("junit_xml: a test case has no name")
	}
	hash := fingerprint(c.suite, c.class, c.name)
	seen[string(hash)]++
	n := seen[string(hash)]
	if n > _maxOrdinal {
		return parsedTest{}, invalidReport("junit_xml: case %.32q is repeated more than %d times", c.name, _maxOrdinal)
	}
	t := parsedTest{key: caseKey(c.suite, c.class, c.name), keyHash: hash, ordinal: n, outcome: entity.TestPassed, durationMS: c.duration}
	switch {
	case c.failed:
		t.outcome = entity.TestFailed
	case c.errored:
		t.outcome = entity.TestError
	case c.skipped:
		t.outcome = entity.TestSkipped
	}
	return t, nil
}

// caseKey is the readable key of a case: the suite, the class unless it repeats the suite, and the name. The hash of
// the three parts is its identity, so a cut key stays unique.
func caseKey(suite, class, name string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{suite, class} {
		if p != "" && (len(parts) == 0 || parts[len(parts)-1] != p) {
			parts = append(parts, p)
		}
	}
	key := strings.Join(append(parts, name), "::")
	if len(key) > _maxCaseKey {
		key = key[:_maxCaseKey]
		for !utf8.ValidString(key) {
			key = key[:len(key)-1]
		}
	}
	return key
}

// milliseconds reads the time of a case in seconds; a time that is not a number of seconds is unknown.
func milliseconds(s string) *int64 {
	sec, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || sec < 0 || sec > _maxDurationS || math.IsNaN(sec) {
		return nil
	}
	ms := int64(math.Round(sec * 1000))
	return &ms
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
