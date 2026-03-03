package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newArchivesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archives",
		Short: "List archived converge graph states",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runArchives(cwd)
		},
	}
}

func runArchives(projectDir string) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	fmt.Println("current\tactive state")
	archives, err := svc.ListArchiveMetadata()
	if err != nil {
		return err
	}
	if len(archives) == 0 {
		fmt.Println("(no archived states yet)")
		return nil
	}

	for _, archive := range archives {
		fmt.Printf(
			"%s\tcommit=%s\tbranch=%s\tcells=%d\tarchived_at=%s\n",
			archive.ArchiveID,
			shortID(archive.CommitSHA),
			archive.Branch,
			archive.CellCount,
			archive.ArchivedAt,
		)
	}
	return nil
}
