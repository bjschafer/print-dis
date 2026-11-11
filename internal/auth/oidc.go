package auth

import (
	"context"
	"errors"

	"github.com/bjschafer/print-dis/internal/config"
)

const keyReturnTo = "return_to"

type oidcClient struct{}

func newOIDCClient(_ context.Context, _ *config.Config) (*oidcClient, error) {
	return nil, errors.New("OIDC sign-in is not built yet; set DEV_USER for local development")
}
