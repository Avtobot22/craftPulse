package normalize

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type testRow struct {
	key     string
	ordinal int
	outcome string
	ms      int64
}

func testRows(tests []parsedTest) []testRow {
	out := make([]testRow, len(tests))
	for i := range tests {
		t := &tests[i]
		out[i] = testRow{key: t.key, ordinal: t.ordinal, outcome: t.outcome, ms: -1}
		if t.durationMS != nil {
			out[i].ms = *t.durationMS
		}
	}
	return out
}

func TestParseJUnit_Dialects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file, version string
		want          []testRow
	}{
		{"junit-gotestsum.xml", "testsuites", []testRow{
			{"example.com/demo/calc::TestDiv", 1, entity.TestPassed, 0},
			{"example.com/demo/calc::TestDivZero", 1, entity.TestPassed, 1},
			{"example.com/demo/calc::TestAbsBroken", 1, entity.TestFailed, 0},
			{"example.com/demo/calc::TestSkipped", 1, entity.TestSkipped, 0},
			{"example.com/demo/calc::TestTable/one", 1, entity.TestPassed, 0},
			{"example.com/demo/calc::TestTable/two", 1, entity.TestPassed, 0},
			{"example.com/demo/calc::TestTable", 1, entity.TestPassed, 0},
		}},
		{"junit-pytest.xml", "testsuites", []testRow{
			{"pytest::tests.test_calc::test_div", 1, entity.TestPassed, 1},
			{"pytest::tests.test_calc::test_div_zero", 1, entity.TestFailed, 2},
			{"pytest::tests.test_calc::test_skip", 1, entity.TestSkipped, 0},
			{"pytest::tests.test_calc::test_fixture", 1, entity.TestError, 0},
			{"pytest::tests.test_calc.TestAbs::test_abs[-1]", 1, entity.TestPassed, 0},
		}},
		{"junit-jest.xml", "testsuites", []testRow{
			{"calc::calc divides::calc divides", 1, entity.TestPassed, 2},
			{"calc::calc rejects zero::calc rejects zero", 1, entity.TestFailed, 3},
			{"calc::calc pending::calc pending", 1, entity.TestSkipped, 0},
		}},
	}
	for _, tt := range cases {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			got, err := parse(t, "junit_xml", entity.AnalysisJUnit, fixture(t, tt.file))
			require.NoError(t, err)
			assert.Equal(t, tt.version, got.formatVersion)
			assert.Equal(t, tt.want, testRows(got.tests))
			assert.Empty(t, got.coverage, "JUnit gives no coverage")
		})
	}
}

func TestParseJUnit_KeysAndOrdinals(t *testing.T) {
	t.Parallel()
	data := []byte(`<testsuite name="s"><testcase classname="c" name="a" time="bad"/><testcase classname="c" name="a" time="-1">` +
		`<failure/><error/></testcase><testcase name="a"/><testsuite name="inner"><testcase classname="c" name="a" time="1.5"/></testsuite></testsuite>`)
	got, err := parse(t, "junit_xml", entity.AnalysisJUnit, data)
	require.NoError(t, err)
	assert.Equal(t, "testsuite", got.formatVersion)
	assert.Equal(t, []testRow{
		{"s::c::a", 1, entity.TestPassed, -1},
		{"s::c::a", 2, entity.TestFailed, -1},
		{"s::a", 1, entity.TestPassed, -1},
		{"inner::c::a", 1, entity.TestPassed, 1500},
	}, testRows(got.tests), "the same case repeats with the next ordinal; the nearest suite names a case")
	assert.Equal(t, got.tests[0].keyHash, got.tests[1].keyHash)
	assert.NotEqual(t, got.tests[0].keyHash, got.tests[2].keyHash, "a case without a class is another case")

	long := strings.Repeat("я", 1500)
	got, err = parse(t, "junit_xml", entity.AnalysisJUnit, []byte(`<testsuites><testsuite name="s"><testcase name="`+long+`"/></testsuite></testsuites>`))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got.tests[0].key), _maxCaseKey)
	assert.True(t, strings.HasPrefix(got.tests[0].key, "s::я"))
}

func TestParseJUnit_Rejected(t *testing.T) {
	t.Parallel()
	gotestsum := fixture(t, "junit-gotestsum.xml")
	cases := []struct {
		name, kind, reason string
		data               []byte
	}{
		{"truncated", entity.AnalysisJUnit, entity.RecordReportInvalid, gotestsum[:len(gotestsum)/2]},
		{"empty file", entity.AnalysisJUnit, entity.RecordReportInvalid, nil},
		{"not xml", entity.AnalysisJUnit, entity.RecordReportInvalid, []byte(`{"tests":[]}`)},
		{"case without name", entity.AnalysisJUnit, entity.RecordReportInvalid, []byte(`<testsuites><testcase classname="c"/></testsuites>`)},
		{"nested case", entity.AnalysisJUnit, entity.RecordReportInvalid, []byte(`<testsuite><testcase name="a"><testcase name="b"/></testcase></testsuite>`)},
		{"NUnit", entity.AnalysisJUnit, entity.RecordReportVersionUnsupported, []byte(`<test-run testcasecount="1"><test-case name="a"/></test-run>`)},
		{"coverage kind", entity.AnalysisCoverage, entity.RecordReportFormatUnsupported, gotestsum},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse(t, "junit_xml", tt.kind, tt.data)
			var rejected *errReport
			require.True(t, errors.As(err, &rejected), "%v", err)
			assert.Equal(t, tt.reason, rejected.reason)
		})
	}
	empty, err := parse(t, "junit_xml", entity.AnalysisJUnit, []byte(`<?xml version="1.0"?><testsuites tests="0"/>`))
	require.NoError(t, err)
	assert.Empty(t, empty.tests, "an empty report has no executions")
}

type coverageRow struct {
	path, kind          string
	covered, executable int
}

func coverageRows(list []parsedCoverage) []coverageRow {
	out := make([]coverageRow, len(list))
	for i, c := range list {
		out[i] = coverageRow{c.path, c.kind, c.covered, c.executable}
	}
	return out
}

func parseCoverage(t *testing.T, format string, data []byte, root, dir string) (*parsedReport, error) {
	t.Helper()
	p, ok := parserOf(format)
	require.True(t, ok)
	return p.read(bytes.NewReader(data), parseInput{kind: entity.AnalysisCoverage, sourceRoot: root, sourceDir: dir})
}

func TestParseCoverage_Dialects(t *testing.T) {
	t.Parallel()
	outside := []entity.SkippedTarget{{Path: "/usr/lib/python3/dist-packages/six.py", Reason: entity.SkippedOutsideRepository}}
	cases := []struct {
		name, format, file, root, dir, version string
		want                                   []coverageRow
		excluded                               []entity.SkippedTarget
	}{
		{"coverage.py Cobertura from sources", "cobertura", "coverage-py.xml", "/builds/demo/app", "", "04", []coverageRow{
			{"src/calc/__init__.py", "line", 0, 0},
			{"src/calc/__init__.py", "branch", 0, 0},
			{"src/calc/ops.py", "line", 2, 4},
			{"src/calc/ops.py", "branch", 1, 2},
			{"src/calc/util.py", "line", 3, 3},
			{"src/calc/util.py", "branch", 0, 0},
		}, outside},
		{"Cobertura without a checkout", "cobertura", "coverage-py.xml", "", "", "04", []coverageRow{}, []entity.SkippedTarget{
			{Path: "/builds/demo/app/src/calc/__init__.py", Reason: entity.SkippedOutsideRepository},
			{Path: "/builds/demo/app/src/calc/ops.py", Reason: entity.SkippedOutsideRepository},
			{Path: "/builds/demo/app/src/calc/util.py", Reason: entity.SkippedOutsideRepository},
			outside[0],
		}},
		{"istanbul LCOV with branches", "lcov", "lcov-istanbul.info", "", "", "tracefile", []coverageRow{
			{"src/calc.js", "line", 4, 5},
			{"src/calc.js", "branch", 2, 4},
			{"src/index.js", "line", 1, 1},
			{"src/index.js", "branch", 0, 0},
		}, nil},
		{"JaCoCo from its source directory", "jacoco", "jacoco.xml", "", "src/main/java", "1.1", []coverageRow{
			{"src/main/java/com/example/calc/Ops.java", "line", 3, 4}, {"src/main/java/com/example/calc/Ops.java", "branch", 1, 2},
		}, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCoverage(t, tt.format, fixture(t, tt.file), tt.root, tt.dir)
			require.NoError(t, err)
			assert.Equal(t, tt.version, got.formatVersion)
			assert.Equal(t, tt.want, coverageRows(got.coverage))
			assert.Equal(t, tt.excluded, got.excluded)
			assert.Empty(t, got.tests, "coverage gives no executions")
		})
	}
}

func TestParseCoverage_LineMaps(t *testing.T) {
	t.Parallel()
	data := []byte("SF:/w/repo/a.go\nDA:3,1\nDA:4,0\nBRDA:3,0,0,2\nBRDA:3,0,1,-\nend_of_record\nSF:a.go\nDA:4,2\nDA:7,0\nBRDA:3,0,1,1\nend_of_record\n")
	got, err := parseCoverage(t, "lcov", data, "/w/repo/", "")
	require.NoError(t, err)
	require.Len(t, got.coverage, 2)
	assert.Equal(t, coverageRow{"a.go", "line", 2, 3}, coverageRows(got.coverage)[0], "records of one file add up")
	assert.Equal(t, [][]int64{{3, 1}, {4, 2}, {7, 0}}, got.coverage[0].points)
	assert.Equal(t, coverageRow{"a.go", "branch", 2, 2}, coverageRows(got.coverage)[1])
	assert.Equal(t, [][]int64{{3, 2, 2}}, got.coverage[1].points)

	noBranches, err := parseCoverage(t, "lcov", []byte("SF:b.go\nDA:1,1\nend_of_record\n"), "", "")
	require.NoError(t, err)
	assert.Equal(t, []coverageRow{{"b.go", "line", 1, 1}}, coverageRows(noBranches.coverage), "LCOV without branches gives no branch row")

	windows, err := parseCoverage(t, "lcov", []byte("SF:C:\\builds\\app\\src\\a.ts\nDA:1,1\nend_of_record\nSF:../other/b.ts\nDA:1,1\nend_of_record\n"), `C:\builds\app`, "")
	require.NoError(t, err)
	assert.Equal(t, []coverageRow{{"src/a.ts", "line", 1, 1}}, coverageRows(windows.coverage))
	assert.Equal(t, []entity.SkippedTarget{{Path: "../other/b.ts", Reason: entity.SkippedOutsideRepository}}, windows.excluded)

	empty, err := parseCoverage(t, "lcov", nil, "", "")
	require.NoError(t, err)
	assert.Empty(t, empty.coverage, "an empty tracefile covers no file")
}

func TestParseCoverage_Rejected(t *testing.T) {
	t.Parallel()
	cobertura := fixture(t, "coverage-py.xml")
	jacoco := fixture(t, "jacoco.xml")
	old := bytes.Replace(cobertura, []byte(`<?xml version="1.0" ?>`),
		[]byte(`<?xml version="1.0" ?><!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-03.dtd">`), 1)
	require.NotEqual(t, cobertura, old)
	cases := []struct {
		name, format, reason string
		data                 []byte
	}{
		{"truncated Cobertura", "cobertura", entity.RecordReportInvalid, cobertura[:len(cobertura)/2]},
		{"Cobertura coverage-03", "cobertura", entity.RecordReportVersionUnsupported, old},
		{"not Cobertura", "cobertura", entity.RecordReportInvalid, jacoco},
		{"conditions above total", "cobertura", entity.RecordReportInvalid, []byte(`<coverage><packages><package><classes><class filename="a.py"><lines>` +
			`<line number="1" hits="1" branch="true" condition-coverage="300% (3/1)"/></lines></class></classes></package></packages></coverage>`)},
		{"branches out of range", "cobertura", entity.RecordReportInvalid, []byte(`<coverage><packages><package><classes><class filename="a.py"><lines>` +
			`<line number="1" hits="1" branch="true" condition-coverage="50% (1/2147483647)"/>` +
			`<line number="2" hits="1" branch="true" condition-coverage="50% (1/2147483647)"/></lines></class></classes></package></packages></coverage>`)},
		{"line number zero", "cobertura", entity.RecordReportInvalid, []byte(`<coverage><packages><package><classes><class filename="a.py"><lines>` +
			`<line number="0" hits="1"/></lines></class></classes></package></packages></coverage>`)},
		{"LCOV without end_of_record", "lcov", entity.RecordReportInvalid, []byte("SF:a.go\nDA:1,1\n")},
		{"LCOV negative hits", "lcov", entity.RecordReportInvalid, []byte("SF:a.go\nDA:1,-1\nend_of_record\n")},
		{"LCOV data outside a record", "lcov", entity.RecordReportInvalid, []byte("DA:1,1\n")},
		{"truncated JaCoCo", "jacoco", entity.RecordReportInvalid, jacoco[:len(jacoco)/2]},
		{"JaCoCo without DOCTYPE", "jacoco", entity.RecordReportInvalid, []byte(`<report name="x"/>`)},
		{"JaCoCo 1.0", "jacoco", entity.RecordReportVersionUnsupported, bytes.Replace(jacoco, []byte("Report 1.1"), []byte("Report 1.0"), 1)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseCoverage(t, tt.format, tt.data, "/builds/demo/app", "")
			var rejected *errReport
			require.True(t, errors.As(err, &rejected), "%v", err)
			assert.Equal(t, tt.reason, rejected.reason)
		})
	}
	_, err := parse(t, "lcov", entity.AnalysisJUnit, []byte("SF:a.go\nend_of_record\n"))
	var rejected *errReport
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, entity.RecordReportFormatUnsupported, rejected.reason, "LCOV carries no test executions")
}
