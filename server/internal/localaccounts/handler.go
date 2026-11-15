package localaccounts

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterPublic registers the unauthenticated local-login endpoint. The
// surrounding HTTP middleware is responsible for identity.WithRequestIP.
func RegisterPublic(api huma.API, service *Service) {
	huma.Register(api, huma.Operation{
		OperationID:  "local-login",
		Method:       http.MethodPost,
		Path:         "/api/auth/local-login",
		Summary:      "Log in with a local account",
		Tags:         []string{"Authentication"},
		MaxBodyBytes: 65536,
	}, func(ctx context.Context, input *localLoginInput) (*localLoginOutput, error) {
		result, cookie, err := service.Login(ctx, input.Body, input.Origin)
		if err != nil {
			return nil, responseError(err)
		}
		return &localLoginOutput{
			CacheControl: "no-store",
			SetCookie:    cookie.String(),
			Body:         result,
		}, nil
	})
}

// RegisterAdmin registers account administration routes. Every service method
// enforces identity.RequireAdmin in addition to any surrounding middleware.
func RegisterAdmin(api huma.API, service *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-local-accounts", Method: http.MethodGet,
		Path: "/api/admin/accounts", Summary: "List local accounts", Tags: []string{"Admin accounts"},
	}, func(ctx context.Context, input *listAccountsInput) (*accountsOutput, error) {
		accounts, err := service.ListAccounts(ctx, input.Limit, input.Offset)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountsOutput{Body: accounts}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-local-account", Method: http.MethodPost,
		Path: "/api/admin/accounts", Summary: "Create a local account", Tags: []string{"Admin accounts"},
		MaxBodyBytes: 65536,
	}, func(ctx context.Context, input *createAccountInput) (*accountOutput, error) {
		account, err := service.CreateAccount(ctx, input.Body)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountOutput{Body: account}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-local-account-profile", Method: http.MethodPost,
		Path: "/api/admin/accounts/{id}/profile", Summary: "Update an account profile", Tags: []string{"Admin accounts"},
		MaxBodyBytes: 65536,
	}, func(ctx context.Context, input *updateProfileInput) (*accountOutput, error) {
		account, err := service.UpdateProfile(ctx, input.ID, input.Body)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountOutput{Body: account}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-local-account-enabled", Method: http.MethodPost,
		Path: "/api/admin/accounts/{id}/enabled", Summary: "Enable or disable an account", Tags: []string{"Admin accounts"},
		MaxBodyBytes: 4096,
	}, func(ctx context.Context, input *setEnabledInput) (*accountOutput, error) {
		account, err := service.SetEnabled(ctx, input.ID, input.Body.Enabled)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountOutput{Body: account}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reset-local-account-sessions", Method: http.MethodPost,
		Path: "/api/admin/accounts/{id}/sessions/reset", Summary: "Revoke account sessions", Tags: []string{"Admin accounts"},
		MaxBodyBytes: 4096,
	}, func(ctx context.Context, input *accountIDInput) (*accountOutput, error) {
		account, err := service.ResetSessions(ctx, input.ID)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountOutput{Body: account}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-local-account-password", Method: http.MethodPost,
		Path: "/api/admin/accounts/{id}/password", Summary: "Replace an account password", Tags: []string{"Admin accounts"},
		MaxBodyBytes: 4096,
	}, func(ctx context.Context, input *setPasswordInput) (*accountOutput, error) {
		account, err := service.SetPassword(ctx, input.ID, input.Body.Password)
		if err != nil {
			return nil, responseError(err)
		}
		return &accountOutput{Body: account}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-local-account-conversations", Method: http.MethodGet,
		Path: "/api/admin/accounts/{id}/conversations", Summary: "List account conversations", Tags: []string{"Admin accounts"},
	}, func(ctx context.Context, input *accountConversationsInput) (*conversationsOutput, error) {
		conversations, err := service.AccountConversations(ctx, input.ID, input.Limit, input.Offset)
		if err != nil {
			return nil, responseError(err)
		}
		return &conversationsOutput{Body: conversations}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-local-account-conversation", Method: http.MethodGet,
		Path: "/api/admin/conversations/{id}", Summary: "Get a conversation with messages", Tags: []string{"Admin accounts"},
	}, func(ctx context.Context, input *accountIDInput) (*conversationOutput, error) {
		conversation, err := service.ConversationForAdmin(ctx, input.ID)
		if err != nil {
			return nil, responseError(err)
		}
		return &conversationOutput{Body: conversation}, nil
	})
}

func responseError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("account.not_found")
	case errors.Is(err, ErrConflict):
		return huma.Error409Conflict("account.username_taken")
	case errors.Is(err, ErrValidation):
		return huma.Error422UnprocessableEntity("account.invalid")
	case errors.Is(err, ErrInvalidOrigin):
		return huma.Error403Forbidden("auth.origin")
	case errors.Is(err, ErrInvalidCredentials):
		return huma.Error401Unauthorized("auth.invalid_credentials")
	case errors.Is(err, ErrAdminAccess):
		return huma.Error403Forbidden("auth.admin_vpn_required")
	case errors.Is(err, ErrLoginThrottled):
		return huma.Error429TooManyRequests("auth.too_many_attempts")
	case errors.Is(err, ErrCannotDisableAdmin):
		return huma.Error409Conflict("account.current_admin_disable")
	case errors.Is(err, ErrStorage):
		return huma.Error503ServiceUnavailable("storage.unavailable")
	}
	var statusError huma.StatusError
	if errors.As(err, &statusError) {
		return statusError
	}
	return huma.Error500InternalServerError("internal.error")
}
