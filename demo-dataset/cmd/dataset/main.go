package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dataset generate|compare|manifest")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	switch args[0] {
	case "generate":
		seed := flags.Uint64("seed", 21, "deterministic seed")
		asOf := flags.String("as-of", "2026-12-01T00:00:00Z", "exclusive UTC end and as-of")
		path := flags.String("out", "model.json", "output model")
		if err := flags.Parse(args[1:]); err != nil {
			return fmt.Errorf("invalid generate arguments")
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected generate arguments")
		}
		model, err := dataset.Generate(*seed, *asOf)
		if err != nil {
			return err
		}
		if err := dataset.WriteJSON(*path, model); err != nil {
			return err
		}
		hash, err := dataset.Hash(model)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "model SHA-256:", hash)
		return err
	case "compare":
		if len(args) != 3 {
			return fmt.Errorf("usage: dataset compare expected.json actual.json")
		}
		var expected, actual dataset.ExpectedFile
		if err := dataset.ReadJSON(args[1], &expected); err != nil {
			return err
		}
		if err := dataset.ReadJSON(args[2], &actual); err != nil {
			return err
		}
		differences, err := dataset.Compare(&expected, &actual)
		if err != nil {
			return err
		}
		for _, difference := range differences {
			if _, err := fmt.Fprintln(output, difference); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(output, "differences: %d\n", len(differences)); err != nil {
			return err
		}
		if len(differences) != 0 {
			return fmt.Errorf("comparison differs")
		}
		return nil
	case "manifest":
		modelPath := flags.String("model", "model.json", "generated model")
		sourcesDir := flags.String("sources", "", "directory of source manifest parts")
		objectsDir := flags.String("objects", "", "directory of per-kind object files")
		path := flags.String("out", "manifest.json", "output manifest")
		if err := flags.Parse(args[1:]); err != nil {
			return fmt.Errorf("invalid manifest arguments")
		}
		if flags.NArg() != 0 || *sourcesDir == "" || *objectsDir == "" {
			return fmt.Errorf("manifest requires --sources and --objects directories")
		}
		var model dataset.Model
		if err := dataset.ReadJSON(*modelPath, &model); err != nil {
			return err
		}
		sources, err := readParts[dataset.SourceManifest](*sourcesDir)
		if err != nil {
			return err
		}
		objects, err := readParts[dataset.ObjectFile](*objectsDir)
		if err != nil {
			return err
		}
		manifest, err := dataset.Assemble(&model, sources, objects)
		if err != nil {
			return err
		}
		return dataset.WriteJSON(*path, manifest)
	default:
		return fmt.Errorf("unknown command; use generate, compare or manifest")
	}
}

func readParts[T any](directory string) ([]T, error) {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("parts directory does not exist")
	}
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("invalid parts directory")
	}
	sort.Strings(paths)
	result := make([]T, 0, len(paths))
	for _, path := range paths {
		var part T
		if err := dataset.ReadJSON(path, &part); err != nil {
			return nil, err
		}
		result = append(result, part)
	}
	return result, nil
}
