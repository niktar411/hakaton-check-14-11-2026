package operations

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"net/http"
)

func Register(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{OperationID: "me", Method: http.MethodGet, Path: "/api/me"}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
		identity, err := s.Identity(ctx)
		if err != nil {
			return nil, err
		}
		return &meOutput{Body: identity}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "dashboard", Method: http.MethodGet, Path: "/api/admin/dashboard"}, func(ctx context.Context, _ *struct{}) (*dashboardOutput, error) {
		out, err := s.Dashboard(ctx)
		if err != nil {
			return nil, err
		}
		return &dashboardOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "usage", Method: http.MethodGet, Path: "/api/admin/usage"}, func(ctx context.Context, in *usageInput) (*usageOutput, error) {
		out, err := s.Usage(ctx, in.Offset)
		if err != nil {
			return nil, err
		}
		return &usageOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "peer-action", Method: http.MethodPost, Path: "/api/admin/peers/{id}"}, func(ctx context.Context, in *peerInput) (*peerOutput, error) {
		out, err := s.ChangePeer(ctx, in.ID, in.Body.Action)
		if err != nil {
			return nil, err
		}
		return &peerOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "update-model-key", Method: http.MethodPut, Path: "/api/admin/model-key", MaxBodyBytes: 8192}, func(ctx context.Context, in *modelKeyInput) (*modelStatusOutput, error) {
		return s.UpdateModelKey(ctx, in)
	})
	huma.Register(api, huma.Operation{OperationID: "mihomo", Method: http.MethodGet, Path: "/api/admin/mihomo"}, func(ctx context.Context, _ *struct{}) (*mihomoOutput, error) {
		return s.Mihomo(ctx)
	})
	huma.Register(api, huma.Operation{OperationID: "chat", Method: http.MethodPost, Path: "/api/chat", MaxBodyBytes: 70000}, func(ctx context.Context, in *chatInput) (*chatOutput, error) {
		out, err := s.Chat(ctx, in)
		if err != nil {
			return nil, err
		}
		return &chatOutput{Body: out}, nil
	})
}
