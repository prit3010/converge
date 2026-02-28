package cli

import (
	"os"
	"strings"

	"github.com/prittamravi/converge/internal/ui"
	"github.com/spf13/cobra"
)

func newUICmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Launch the local web dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runUI(cwd, addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7777", "Address to bind the UI server")
	return cmd
}

func runUI(projectDir, addr string) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}

	server, err := ui.NewServer(svc)
	if err != nil {
		_ = svc.DB.Close()
		return err
	}

	defer svc.DB.Close()
	return server.ListenAndServe(strings.TrimSpace(addr))
}
