package grpc

import (
	"context"

	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/usecase"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	reviewv1.UnimplementedReviewServiceServer
	uc *usecase.ReviewUseCase
}

func NewServer(uc *usecase.ReviewUseCase) *Server {
	return &Server{uc: uc}
}

func (s *Server) CreateReview(ctx context.Context, req *reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
	review, err := s.uc.CreateReview(ctx, usecase.CreateReviewInput{
		UserID:      req.GetUserId(),
		UserName:    req.GetUserName(),
		VenueID:     req.GetVenueId(),
		MasterID:    req.GetMasterId(),
		Rating:      req.GetRating(),
		Text:        req.GetText(),
		IsAnonymous: req.GetIsAnonymous(),
	})
	if err != nil {
		return nil, err
	}
	return reviewToProto(review), nil
}

func (s *Server) GetReview(ctx context.Context, req *reviewv1.GetReviewRequest) (*reviewv1.ReviewResponse, error) {
	review, err := s.uc.GetReview(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return reviewToProto(review), nil
}

func (s *Server) ListVenueReviews(ctx context.Context, req *reviewv1.ListVenueReviewsRequest) (*reviewv1.ListReviewsResponse, error) {
	reviews, total, err := s.uc.ListVenueReviews(ctx, req.GetVenueId(), req.GetOnlyUnanswered(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}

	pbReviews := make([]*reviewv1.ReviewResponse, 0, len(reviews))
	for _, r := range reviews {
		pbReviews = append(pbReviews, reviewToProto(r))
	}
	return &reviewv1.ListReviewsResponse{
		Reviews: pbReviews,
		Total:   total,
	}, nil
}

func (s *Server) GetVenueRating(ctx context.Context, req *reviewv1.GetVenueRatingRequest) (*reviewv1.VenueRatingResponse, error) {
	vr, err := s.uc.GetVenueRating(ctx, req.GetVenueId())
	if err != nil {
		return nil, err
	}
	return &reviewv1.VenueRatingResponse{
		VenueId:     vr.VenueID,
		AvgRating:   vr.AvgRating,
		ReviewCount: vr.ReviewCount,
	}, nil
}

func (s *Server) GetMasterRating(ctx context.Context, req *reviewv1.GetMasterRatingRequest) (*reviewv1.MasterRatingResponse, error) {
	mr, err := s.uc.GetMasterRating(ctx, req.GetMasterId())
	if err != nil {
		return nil, err
	}
	return &reviewv1.MasterRatingResponse{
		MasterId:    mr.MasterID,
		AvgRating:   mr.AvgRating,
		ReviewCount: mr.ReviewCount,
	}, nil
}

func (s *Server) ListMasterReviews(ctx context.Context, req *reviewv1.ListMasterReviewsRequest) (*reviewv1.ListReviewsResponse, error) {
	reviews, total, err := s.uc.ListMasterReviews(ctx, req.GetMasterId(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}

	pbReviews := make([]*reviewv1.ReviewResponse, 0, len(reviews))
	for _, r := range reviews {
		pbReviews = append(pbReviews, reviewToProto(r))
	}
	return &reviewv1.ListReviewsResponse{
		Reviews: pbReviews,
		Total:   total,
	}, nil
}

func (s *Server) ReplyToReview(ctx context.Context, req *reviewv1.ReplyToReviewRequest) (*reviewv1.ReviewReply, error) {
	reply, err := s.uc.ReplyToReview(ctx, req.GetReviewId(), req.GetVenueId(), req.GetAuthorUserId(), req.GetBody())
	if err != nil {
		return nil, err
	}
	return replyToProto(reply), nil
}

func (s *Server) DeleteReviewReply(ctx context.Context, req *reviewv1.DeleteReviewReplyRequest) (*reviewv1.DeleteReviewReplyResponse, error) {
	if err := s.uc.DeleteReviewReply(ctx, req.GetReviewId(), req.GetVenueId()); err != nil {
		return nil, err
	}
	return &reviewv1.DeleteReviewReplyResponse{}, nil
}

func (s *Server) GetVenueReviewSummary(ctx context.Context, req *reviewv1.GetVenueReviewSummaryRequest) (*reviewv1.VenueReviewSummaryResponse, error) {
	sum, err := s.uc.GetVenueReviewSummary(ctx, req.GetVenueId())
	if err != nil {
		return nil, err
	}
	return &reviewv1.VenueReviewSummaryResponse{
		VenueId:         sum.VenueID,
		AvgRating:       sum.AvgRating,
		ReviewCount:     sum.ReviewCount,
		UnansweredCount: sum.UnansweredCount,
	}, nil
}

func reviewToProto(r *domain.Review) *reviewv1.ReviewResponse {
	return &reviewv1.ReviewResponse{
		Id:          r.ID,
		UserId:      r.UserID,
		UserName:    r.UserName,
		VenueId:     r.VenueID,
		MasterId:    r.MasterID,
		Rating:      r.Rating,
		Text:        r.Text,
		IsVerified:  r.IsVerified,
		IsAnonymous: r.IsAnonymous,
		CreatedAt:   timestamppb.New(r.CreatedAt),
		OwnerReply:  replyToProto(r.Reply),
	}
}

func replyToProto(r *domain.ReviewReply) *reviewv1.ReviewReply {
	if r == nil {
		return nil
	}
	return &reviewv1.ReviewReply{
		Body:      r.Body,
		CreatedAt: timestamppb.New(r.CreatedAt),
		UpdatedAt: timestamppb.New(r.UpdatedAt),
	}
}
