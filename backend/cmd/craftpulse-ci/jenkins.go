package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
)

func (a *app) jenkinsCmd() *cobra.Command {
	var step ci.JenkinsStep
	var dir, started, finished, output string
	cmd := &cobra.Command{Use: "jenkins", Short: "record attempts and finish the Jenkins execution manifest"}
	record := &cobra.Command{
		Use: "record", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			step.Started, err = time.Parse(time.RFC3339, started)
			if err != nil {
				return errors.New("--started-at must be RFC3339")
			}
			step.Finished = time.Now()
			if finished != "" {
				step.Finished, err = time.Parse(time.RFC3339, finished)
				if err != nil {
					return errors.New("--finished-at must be RFC3339")
				}
			}
			key, err := step.Record(a.getenv, dir)
			if err == nil {
				fmt.Fprintln(a.stdout, key)
			}
			return err
		},
	}
	record.Flags().StringVar(&step.Stage, "stage", "", "Jenkins stage name")
	record.Flags().StringVar(&step.Name, "step", "", "logical step name")
	record.Flags().IntVar(&step.Attempt, "attempt", 1, "retry number, starting at 1")
	record.Flags().StringVar(&step.Outcome, "outcome", "", "execution outcome")
	record.Flags().StringVar(&step.Agent, "agent", a.getenv("NODE_NAME"), "agent label")
	record.Flags().StringToStringVar(&step.Axes, "axes", map[string]string{}, "matrix axes as key=value pairs")
	record.Flags().StringVar(&started, "started-at", "", "attempt start time in RFC3339")
	record.Flags().StringVar(&finished, "finished-at", "", "attempt finish time in RFC3339 (defaults to now)")
	finish := &cobra.Command{
		Use: "finish", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return ci.FinishJenkinsExecution(a.getenv, dir, output)
		},
	}
	finish.Flags().StringVar(&output, "output", "execution-manifest.json", "artifact path")
	cmd.PersistentFlags().StringVar(&dir, "attempts-dir", "", "build-specific directory shared by all attempts")
	_ = cmd.MarkPersistentFlagRequired("attempts-dir")
	cmd.AddCommand(record, finish)
	return cmd
}
