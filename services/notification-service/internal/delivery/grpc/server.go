package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
	"github.com/tienlao/agregator/services/notification-service/internal/usecase"
)

const internalErrMsg = "internal error"

// Server implements notificationv1.NotificationServiceServer.
type Server struct {
	notificationv1.UnimplementedNotificationServiceServer
	uc  *usecase.NotificationUseCase
	log zerolog.Logger
}

func NewServer(uc *usecase.NotificationUseCase, log zerolog.Logger) *Server {
	return &Server{uc: uc, log: log}
}

// passthroughOrInternal returns err unchanged when it already carries a gRPC
// status code (set by the usecase), otherwise logs and returns Internal so
// implementation details never leak to callers.
func (s *Server) passthroughOrInternal(err error, msg string) error {
	if _, ok := status.FromError(err); ok {
		return err
	}
	s.log.Error().Err(err).Msg(msg)
	return pkgerrors.Internal(internalErrMsg)
}

func parseUUIDArg(raw, field string) (uuid.UUID, error) {
	u, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, pkgerrors.InvalidArgument("invalid " + field)
	}
	return u, nil
}

func toPB(n *domain.Notification) *notificationv1.Notification {
	if n == nil {
		return nil
	}
	out := &notificationv1.Notification{
		Id:        n.ID.String(),
		UserId:    n.UserID.String(),
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		Data:      n.Data,
		Read:      n.ReadAt != nil,
		CreatedAt: timestamppb.New(n.CreatedAt),
	}
	if n.ReadAt != nil {
		out.ReadAt = timestamppb.New(*n.ReadAt)
	}
	return out
}

func (s *Server) Create(ctx context.Context, req *notificationv1.CreateRequest) (*notificationv1.CreateResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	n, err := s.uc.Create(ctx, userID, req.GetType(), req.GetTitle(), req.GetBody(), req.GetData())
	if err != nil {
		return nil, s.passthroughOrInternal(err, "create notification")
	}
	return &notificationv1.CreateResponse{Notification: toPB(n)}, nil
}

func (s *Server) List(ctx context.Context, req *notificationv1.ListRequest) (*notificationv1.ListResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	list, total, unread, err := s.uc.List(ctx, userID, req.GetLimit(), req.GetOffset(), req.GetUnreadOnly())
	if err != nil {
		return nil, s.passthroughOrInternal(err, "list notifications")
	}
	out := make([]*notificationv1.Notification, 0, len(list))
	for i := range list {
		out = append(out, toPB(&list[i]))
	}
	return &notificationv1.ListResponse{Notifications: out, Total: total, UnreadCount: unread}, nil
}

func (s *Server) GetUnreadCount(ctx context.Context, req *notificationv1.GetUnreadCountRequest) (*notificationv1.GetUnreadCountResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	c, err := s.uc.UnreadCount(ctx, userID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "unread count")
	}
	return &notificationv1.GetUnreadCountResponse{UnreadCount: c}, nil
}

func (s *Server) MarkRead(ctx context.Context, req *notificationv1.MarkReadRequest) (*notificationv1.MarkReadResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	nID, err := parseUUIDArg(req.GetNotificationId(), "notification_id")
	if err != nil {
		return nil, err
	}
	c, err := s.uc.MarkRead(ctx, userID, nID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "mark read")
	}
	return &notificationv1.MarkReadResponse{UnreadCount: c}, nil
}

func (s *Server) MarkAllRead(ctx context.Context, req *notificationv1.MarkAllReadRequest) (*notificationv1.MarkAllReadResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	c, err := s.uc.MarkAllRead(ctx, userID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "mark all read")
	}
	return &notificationv1.MarkAllReadResponse{UnreadCount: c}, nil
}
