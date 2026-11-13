// Command envelope checks a canonical fact package from a file: envelope check <package.tar.gz>.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Exit codes: 1 - the package is rejected as a whole or the command failed, 2 - some records are rejected.
const (
	_exitRejected        = 1
	_exitRecordsRejected = 2
)

var errUsage = errors.New("usage: envelope check <package.tar.gz>")

func main() {
	code, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "envelope:", err)
	}
	os.Exit(code)
}

func run(args []string, out io.Writer) (int, error) {
	if len(args) != 2 || args[0] != "check" {
		return _exitRejected, errUsage
	}
	f, err := os.Open(args[1]) //nolint:gosec // the operator names the package file to check
	if err != nil {
		return _exitRejected, err
	}
	defer f.Close()
	result := envelope.Check(f, envelope.DefaultLimits)
	if err := report(out, result); err != nil {
		return _exitRejected, err
	}
	switch {
	case result.Envelope != nil:
		return _exitRejected, nil
	case result.Rejected() > 0:
		return _exitRecordsRejected, nil
	}
	return 0, nil
}

func report(out io.Writer, result *envelope.Result) error {
	if m := result.Manifest; m != nil {
		if _, err := fmt.Fprintf(out, "manifest: %s/%s from %s %s\n", m.SchemaId, m.SchemaVersion, m.Source.Provider, m.Source.InstanceKey); err != nil {
			return err
		}
	}
	if result.Envelope != nil {
		_, err := fmt.Fprintf(out, "package rejected: %s\n", result.Envelope)
		return err
	}
	for _, rec := range result.Records {
		line := fmt.Sprintf("record %d accepted %s %s sha256=%s", rec.No, rec.Kind, key(rec), rec.Hash)
		if rec.Rejection != nil {
			line = fmt.Sprintf("record %d rejected %s %s %s", rec.No, rec.Kind, key(rec), rec.Rejection)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	rejected := result.Rejected()
	_, err := fmt.Fprintf(out, "records: %d accepted, %d rejected\n", len(result.Records)-rejected, rejected)
	return err
}

// key prints the source identity of a record: native_ns/native_id, or the ID alone in an empty namespace.
func key(rec envelope.RecordResult) string {
	switch {
	case rec.NativeID == "":
		return "(no native_id)"
	case rec.NativeNS == "":
		return rec.NativeID
	}
	return rec.NativeNS + "/" + rec.NativeID
}
