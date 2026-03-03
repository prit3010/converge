package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prit3010/converge/internal/core"
	"github.com/spf13/cobra"
)

type hookGitCommitFlags struct {
	SHA     string
	Branch  string
	Subject string
}

func newHookGitCommitCmd() *cobra.Command {
	flags := &hookGitCommitFlags{}
	cmd := &cobra.Command{
		Use:   "git-commit",
		Short: "Archive active state and create a fresh baseline after a git commit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runHookGitCommit(cwd, *flags, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&flags.SHA, "sha", "", "Commit SHA (defaults to HEAD)")
	cmd.Flags().StringVar(&flags.Branch, "branch", "", "Commit branch name")
	cmd.Flags().StringVar(&flags.Subject, "subject", "", "Commit subject line")
	return cmd
}

func runHookGitCommit(projectDir string, flags hookGitCommitFlags, out io.Writer) error {
	sha := strings.TrimSpace(flags.SHA)
	var err error
	if sha == "" {
		sha, err = runGitCommand(projectDir, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
	}
	branch := strings.TrimSpace(flags.Branch)
	if branch == "" {
		branch, err = runGitCommand(projectDir, "branch", "--show-current")
		if err != nil {
			branch = "detached"
		}
	}
	subject := strings.TrimSpace(flags.Subject)
	if subject == "" {
		subject, err = runGitCommand(projectDir, "log", "-1", "--pretty=%s", sha)
		if err != nil {
			subject = "(no subject)"
		}
	}
	committedAt, err := runGitCommand(projectDir, "log", "-1", "--format=%cI", sha)
	if err != nil || strings.TrimSpace(committedAt) == "" {
		committedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer func() {
		if svc.DB != nil {
			_ = svc.DB.Close()
		}
	}()

	result, rotateErr := svc.RotateOnGitCommit(context.Background(), core.GitCommitMetadata{
		SHA:         sha,
		Branch:      branch,
		Subject:     subject,
		CommittedAt: committedAt,
	})
	if rotateErr != nil {
		replay := fmt.Sprintf(
			"converge hook git-commit --sha %s --branch %s --subject %s",
			strconv.Quote(sha),
			strconv.Quote(branch),
			strconv.Quote(subject),
		)
		return fmt.Errorf("git-commit hook failed: %w\nreplay: %s", rotateErr, replay)
	}

	if result.Archive == nil {
		fmt.Fprintf(out, "archive_skipped=empty commit=%s branch=%s\n", shortID(sha), branch)
	} else {
		fmt.Fprintf(out, "archived=%s commit=%s branch=%s cells=%d\n", result.Archive.ArchiveID, shortID(sha), branch, result.Archive.CellCount)
	}
	fmt.Fprintf(out, "baseline=%s source=%s\n", result.BaselineCell.ID, result.BaselineCell.Source)
	return nil
}

func shortID(in string) string {
	in = strings.TrimSpace(in)
	if len(in) <= 8 {
		return in
	}
	return in[:8]
}
