package userservice

import (
	"context"
	"fmt"
	"sync"

	"velocity/internal/persistence/postgres/generated"
	"velocity/internal/persistence/postgres/repository"
	"velocity/internal/service/walletservice"
	"velocity/pkg/logger"
	"velocity/pkg/timeutil"
)

type Service struct {
	userRepo      repository.UserRepository
	walletSvc     *walletservice.Service
	verifiedUsers sync.Map
}

func New(
	userRepo repository.UserRepository,
	walletSvc *walletservice.Service,
) *Service {
	return &Service{
		userRepo:  userRepo,
		walletSvc: walletSvc,
	}
}

func (s *Service) CreateUser(
	ctx context.Context,
	req CreateUserRequest,
) (*generated.User, error) {

	if req.ID <= 0 {
		return nil, fmt.Errorf("invalid user id: %d", req.ID)
	}

	if _, cached := s.verifiedUsers.Load(req.ID); cached {
		user, err := s.userRepo.GetByID(ctx, req.ID)
		if err == nil {
			return &user, nil
		}
	}

	exists, err := s.userRepo.Exists(ctx, req.ID)
	if err != nil {
		logger.Error(
			"failed checking user existence",
			logger.Int64("user_id", req.ID),
			logger.ErrorField(err),
		)
		return nil, err
	}

	if exists {
		user, err := s.userRepo.GetByID(ctx, req.ID)
		if err != nil {
			return nil, err
		}

		_ = s.walletSvc.CreateDefaultWallets(ctx, user.ID)
		s.verifiedUsers.Store(req.ID, true)
		return &user, nil
	}

	user, err := s.userRepo.Create(
		ctx,
		generated.CreateUserParams{
			ID:        req.ID,
			Email:     req.Email,
			CreatedAt: timeutil.UTCNow(),
			UpdatedAt: timeutil.UTCNow(),
		},
	)
	if err != nil {
		// Fallback: check if created concurrently
		if u, getErr := s.userRepo.GetByID(ctx, req.ID); getErr == nil {
			_ = s.walletSvc.CreateDefaultWallets(ctx, u.ID)
			s.verifiedUsers.Store(req.ID, true)
			return &u, nil
		}

		logger.Error(
			"failed creating user",
			logger.Int64("user_id", req.ID),
			logger.ErrorField(err),
		)
		return nil, err
	}

	if err := s.walletSvc.CreateDefaultWallets(
		ctx,
		user.ID,
	); err != nil {
		logger.Error(
			"failed creating default wallets",
			logger.ErrorField(err),
		)
	}

	s.verifiedUsers.Store(req.ID, true)

	logger.Info(
		"user synchronized successfully",
		logger.Int64("user_id", user.ID),
		logger.String("email", user.Email),
	)

	return &user, nil
}
