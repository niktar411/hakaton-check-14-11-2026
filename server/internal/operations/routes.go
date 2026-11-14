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
	huma.Register(api, huma.Operation{OperationID: "admin-chat-context", Method: http.MethodGet, Path: "/api/admin/chat-context"}, func(ctx context.Context, _ *struct{}) (*chatContextOutput, error) {
		return s.ChatContext(ctx)
	})
	huma.Register(api, huma.Operation{OperationID: "update-admin-chat-context", Method: http.MethodPut, Path: "/api/admin/chat-context", MaxBodyBytes: 40000}, func(ctx context.Context, in *chatContextUpdateInput) (*chatContextOutput, error) {
		return s.UpdateChatContext(ctx, in)
	})
	huma.Register(api, huma.Operation{OperationID: "chat", Method: http.MethodPost, Path: "/api/chat", MaxBodyBytes: 70000}, func(ctx context.Context, in *chatInput) (*chatOutput, error) {
		out, err := s.Chat(ctx, in)
		if err != nil {
			return nil, err
		}
		return &chatOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-conversations", Method: http.MethodGet, Path: "/api/chat/conversations"}, func(ctx context.Context, in *chatConversationsInput) (*chatConversationsOutput, error) {
		out, err := s.ChatConversations(ctx, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &chatConversationsOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-agent", Method: http.MethodPost, Path: "/api/chat/agent", MaxBodyBytes: 20000}, func(ctx context.Context, in *chatAgentInput) (*chatAgentOutput, error) {
		return s.ChatAgent(ctx, in)
	})
	huma.Register(api, huma.Operation{OperationID: "chat-history", Method: http.MethodGet, Path: "/api/chat/conversations/{id}"}, func(ctx context.Context, in *chatHistoryInput) (*chatHistoryOutput, error) {
		out, err := s.ChatHistory(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		return &chatHistoryOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-updates", Method: http.MethodGet, Path: "/api/chat/updates"}, func(ctx context.Context, _ *struct{}) (*chatUpdatesOutput, error) {
		out, err := s.ChatUpdates(ctx)
		if err != nil {
			return nil, err
		}
		return &chatUpdatesOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-update-pull", Method: http.MethodPost, Path: "/api/chat/updates/{id}/pull"}, func(ctx context.Context, in *chatUpdatePullInput) (*chatUpdatePullOutput, error) {
		out, err := s.PullChatUpdate(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		return &chatUpdatePullOutput{Body: *out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-notifications", Method: http.MethodGet, Path: "/api/chat/notifications"}, func(ctx context.Context, in *chatNotificationsInput) (*chatNotificationsOutput, error) {
		out, err := s.ChatNotifications(ctx, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &chatNotificationsOutput{Body: out}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "chat-notification-ack", Method: http.MethodPost, Path: "/api/chat/notifications/{id}/ack"}, func(ctx context.Context, in *chatNotificationAckInput) (*chatNotificationAckOutput, error) {
		return s.AcknowledgeChatNotification(ctx, in.ID)
	})
}
