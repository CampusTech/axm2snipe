package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// NewAccessTokenCmd creates the access-token command.
func NewAccessTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "access-token",
		Short: "Print an ABM API access token",
		Long:  "Generates and prints an OAuth2 access token for the Apple Business Manager API. Useful for manual API testing with curl.",
		RunE:  runAccessToken,
	}
}

func runAccessToken(cmd *cobra.Command, args []string) error {
	if err := Cfg.ValidateABM(); err != nil {
		return err
	}

	ctx := context.Background()

	ts, err := newABMTokenSource(ctx)
	if err != nil {
		return err
	}

	token, err := ts.Token()
	if err != nil {
		return fmt.Errorf("fetching token: %w", err)
	}

	fmt.Println(token.AccessToken)
	return nil
}
