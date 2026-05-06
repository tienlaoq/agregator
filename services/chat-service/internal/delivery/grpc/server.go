package grpc

import (
	"context"

	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
	"github.com/tienlao/agregator/services/chat-service/internal/usecase"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	chatv1.UnimplementedChatServiceServer
	uc *usecase.ChatUseCase
}

func NewServer(uc *usecase.ChatUseCase) *Server {
	return &Server{uc: uc}
}

func toThreadPB(t *domain.Thread) *chatv1.ChatThread {
	if t == nil {
		return nil
	}
	out := &chatv1.ChatThread{
		Id:                 t.ID.String(),
		Kind:               t.Kind,
		RefId:              t.RefID.String(),
		ParticipantUserIds: append([]string(nil), t.ParticipantUserIDs...),
		UnreadCount:        t.UnreadCount,
	}
	if t.LastMessageID != nil {
		out.LastMessageId = t.LastMessageID.String()
	}
	if t.LastMessageAt != nil {
		out.LastMessageAt = timestamppb.New(*t.LastMessageAt)
	}
	for _, pr := range t.ParticipantReads {
		pb := &chatv1.ParticipantReadState{UserId: pr.UserID}
		if pr.LastReadMessageID != nil {
			pb.LastReadMessageId = pr.LastReadMessageID.String()
		}
		out.ParticipantReads = append(out.ParticipantReads, pb)
	}
	return out
}

func toMessagePB(m *domain.Message) *chatv1.ChatMessage {
	if m == nil {
		return nil
	}
	return &chatv1.ChatMessage{
		Id:           m.ID.String(),
		ThreadId:     m.ThreadID.String(),
		AuthorUserId: m.AuthorUserID.String(),
		Text:         m.Text,
		ClientMsgId:  m.ClientMsgID,
		CreatedAt:    timestamppb.New(m.CreatedAt),
	}
}

func (s *Server) EnsureThread(ctx context.Context, req *chatv1.EnsureThreadRequest) (*chatv1.ThreadResponse, error) {
	t, err := s.uc.EnsureThread(ctx, req.GetKind(), req.GetRefId(), req.GetParticipantUserIds())
	if err != nil {
		return nil, err
	}
	return &chatv1.ThreadResponse{Thread: toThreadPB(t)}, nil
}

func (s *Server) ListThreads(ctx context.Context, req *chatv1.ListThreadsRequest) (*chatv1.ListThreadsResponse, error) {
	list, total, err := s.uc.ListThreads(ctx, req.GetUserId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.ChatThread, 0, len(list))
	for i := range list {
		out = append(out, toThreadPB(&list[i]))
	}
	return &chatv1.ListThreadsResponse{Threads: out, Total: total}, nil
}

func (s *Server) ListMessages(ctx context.Context, req *chatv1.ListMessagesRequest) (*chatv1.ListMessagesResponse, error) {
	list, total, err := s.uc.ListMessages(ctx, req.GetThreadId(), req.GetUserId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.ChatMessage, 0, len(list))
	for i := range list {
		out = append(out, toMessagePB(&list[i]))
	}
	return &chatv1.ListMessagesResponse{Messages: out, Total: total}, nil
}

func (s *Server) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.MessageResponse, error) {
	m, t, err := s.uc.SendMessage(ctx, req.GetThreadId(), req.GetUserId(), req.GetText(), req.GetClientMsgId())
	if err != nil {
		return nil, err
	}
	return &chatv1.MessageResponse{
		Message: toMessagePB(m),
		Thread:  toThreadPB(t),
	}, nil
}

func (s *Server) MarkRead(ctx context.Context, req *chatv1.MarkReadRequest) (*chatv1.ThreadResponse, error) {
	t, err := s.uc.MarkRead(ctx, req.GetThreadId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &chatv1.ThreadResponse{Thread: toThreadPB(t)}, nil
}
